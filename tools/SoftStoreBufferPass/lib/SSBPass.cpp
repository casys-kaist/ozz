#include "SSBPass.h"

#include "llvm/ADT/Statistic.h"
#include "llvm/Analysis/CaptureTracking.h"
#include "llvm/Analysis/ValueTracking.h"
#include "llvm/CodeGen/RegAllocRegistry.h"
#include "llvm/IR/IRBuilder.h"
#include "llvm/IR/LegacyPassManager.h"
#include "llvm/Passes/PassBuilder.h"
#include "llvm/Passes/PassPlugin.h"
#include "llvm/ProfileData/InstrProf.h"
#include "llvm/Support/raw_ostream.h"
#include "llvm/Transforms/IPO/PassManagerBuilder.h"
#include "llvm/Transforms/Utils/Local.h"

using namespace llvm;

#define DEBUG_TYPE "ssb"

static cl::opt<bool> ClBuileKernel("ssb-kernel",
                                   cl::desc("Build a Linux kernel"), cl::Hidden,
                                   cl::init(false));

STATISTIC(NumInstrumentedReads, "Number of instrumented reads");
STATISTIC(NumInstrumentedWrites, "Number of instrumented writes");

namespace {

/*
 *Pass Implementation
 */
struct SoftStoreBuffer {
  bool instrumentFunction(Function &F, const TargetLibraryInfo &TLI);

private:
  void initialize(Module &M);
  bool instrumentLoadOrStore(Instruction *I, const DataLayout &DL);
  FunctionCallee findCallbackFunction();
  bool addrPointsToConstantData(Value *Addr);
  void chooseInstructionsToInstrument(SmallVectorImpl<Instruction *> &Local,
                                      SmallVectorImpl<Instruction *> &All,
                                      const DataLayout &DL);
  // Collected instructions
  SmallVector<Instruction *, 8> AllLoadsAndStores;
  SmallVector<Instruction *, 8> LocalLoadsAndStores;
  SmallVector<Instruction *, 8> AtomicAccesses;
  SmallVector<Instruction *, 8> MemIntrinCalls;
  // Callbacks
  FunctionCallee SSBRead;
  FunctionCallee SSBWrite;
};

// Do not instrument known races/"benign races" that come from compiler
// instrumentatin. The user has no way of suppressing them.
static bool shouldInstrumentReadWriteFromAddress(const Module *M, Value *Addr) {
  // Peel off GEPs and BitCasts.
  Addr = Addr->stripInBoundsOffsets();

  if (GlobalVariable *GV = dyn_cast<GlobalVariable>(Addr)) {
    if (GV->hasSection()) {
      StringRef SectionName = GV->getSection();
      // Check if the global is in the PGO counters section.
      auto OF = Triple(M->getTargetTriple()).getObjectFormat();
      if (SectionName.endswith(
              getInstrProfSectionName(IPSK_cnts, OF, /*AddSegmentInfo=*/false)))
        return false;
    }

    // Check if the global is private gcov data.
    if (GV->getName().startswith("__llvm_gcov") ||
        GV->getName().startswith("__llvm_gcda"))
      return false;
  }

  // Do not instrument acesses from different address spaces; we cannot deal
  // with them.
  if (Addr) {
    Type *PtrTy = cast<PointerType>(Addr->getType()->getScalarType());
    if (PtrTy->getPointerAddressSpace() != 0)
      return false;
  }

  return true;
}

bool SoftStoreBuffer::addrPointsToConstantData(Value *Addr) {
  // If this is a GEP, just analyze its pointer operand.
  if (GetElementPtrInst *GEP = dyn_cast<GetElementPtrInst>(Addr))
    Addr = GEP->getPointerOperand();

  if (GlobalVariable *GV = dyn_cast<GlobalVariable>(Addr)) {
    if (GV->isConstant()) {
      // Reads from constant globals can not race with any writes.
      return true;
    }
  }
  return false;
}

void SoftStoreBuffer::chooseInstructionsToInstrument(
    SmallVectorImpl<Instruction *> &Local, SmallVectorImpl<Instruction *> &All,
    const DataLayout &DL) {
  SmallPtrSet<Value *, 8> WriteTargets;
  // Iterate from the end.
  for (Instruction *I : reverse(Local)) {
    if (StoreInst *Store = dyn_cast<StoreInst>(I)) {
      Value *Addr = Store->getPointerOperand();
      if (!shouldInstrumentReadWriteFromAddress(I->getModule(), Addr))
        continue;
      WriteTargets.insert(Addr);
    } else {
      LoadInst *Load = cast<LoadInst>(I);
      Value *Addr = Load->getPointerOperand();
      if (!shouldInstrumentReadWriteFromAddress(I->getModule(), Addr))
        continue;
      if (addrPointsToConstantData(Addr))
        // Addr points to some constant data -- it can not race with any
        // writes.
        continue;
    }
    Value *Addr = isa<StoreInst>(*I) ? cast<StoreInst>(I)->getPointerOperand()
                                     : cast<LoadInst>(I)->getPointerOperand();
    if (isa<AllocaInst>(GetUnderlyingObject(Addr, DL)) &&
        !PointerMayBeCaptured(Addr, true, true)) {
      // The variable is addressable but not captured, so it cannot be
      // referenced from a different thread and participate in a data race
      // (see llvm/Analysis/CaptureTracking.h for details).
      continue;
    }
    All.push_back(I);
  }
  Local.clear();
}

static bool isAtomic(Instruction *I) {
  // TODO: Ask TTI whether synchronization scope is between threads.
  if (LoadInst *LI = dyn_cast<LoadInst>(I))
    return LI->isAtomic() && LI->getSyncScopeID() != SyncScope::SingleThread;
  if (StoreInst *SI = dyn_cast<StoreInst>(I))
    return SI->isAtomic() && SI->getSyncScopeID() != SyncScope::SingleThread;
  if (isa<AtomicRMWInst>(I))
    return true;
  if (isa<AtomicCmpXchgInst>(I))
    return true;
  if (isa<FenceInst>(I))
    return true;
  return false;
}

bool SoftStoreBuffer::instrumentFunction(Function &F,
                                         const TargetLibraryInfo &TLI) {
  initialize(*F.getParent());

  LLVM_DEBUG(dbgs() << "Instrumenting a function " << F.getName() << "\n");
  bool Res = false;
  bool HasCalls = false;
  bool SanitizeFunction = F.hasFnAttribute(Attribute::SoftStoreBuffer);
  const DataLayout &DL = F.getParent()->getDataLayout();

  // Visiting and cheking all instructions
  for (auto &BB : F) {
    for (auto &Inst : BB) {
      if (isAtomic(&Inst))
        AtomicAccesses.push_back(&Inst);
      else if (isa<LoadInst>(Inst) || isa<StoreInst>(Inst))
        LocalLoadsAndStores.push_back(&Inst);
      else if (isa<CallInst>(Inst) || isa<InvokeInst>(Inst)) {
        if (CallInst *CI = dyn_cast<CallInst>(&Inst))
          maybeMarkSanitizerLibraryCallNoBuiltin(CI, &TLI);
        if (isa<MemIntrinsic>(Inst))
          MemIntrinCalls.push_back(&Inst);
        HasCalls = true;
        chooseInstructionsToInstrument(LocalLoadsAndStores, AllLoadsAndStores,
                                       DL);
      }
    }
    chooseInstructionsToInstrument(LocalLoadsAndStores, AllLoadsAndStores, DL);
  }

  // We have collected all loads and stores.
  if (SanitizeFunction)
    for (auto Inst : AllLoadsAndStores) {
      Res |= instrumentLoadOrStore(Inst, DL);
    }

  Res |= HasCalls;
  return Res;
}

bool SoftStoreBuffer::instrumentLoadOrStore(Instruction *I,
                                            const DataLayout &DL) {
  IRBuilder<> IRB(I);
  bool IsWrite = isa<StoreInst>(*I);
  Value *Addr = IsWrite ? cast<StoreInst>(I)->getPointerOperand()
                        : cast<LoadInst>(I)->getPointerOperand();
  FunctionCallee OnAccessFunc = nullptr;

  // swifterror memory addresses are mem2reg promoted by instruction selection.
  // As such they cannot have regular uses like an instrumentation function and
  // it makes no sense to track them as memory.
  if (Addr->isSwiftError())
    return false;

  LLVM_DEBUG(dbgs() << "Instrumenting a callback at " << *I << "\n");

  // int Idx = getMemoryAccessFuncIndex(Addr, DL);
  // if (Idx < 0)
  //   return false;
  // const unsigned Alignment = IsWrite
  //     ? cast<StoreInst>(I)->getAlignment()
  //     : cast<LoadInst>(I)->getAlignment();
  // const bool IsVolatile =
  //     ClDistinguishVolatile && (IsWrite ? cast<StoreInst>(I)->isVolatile()
  //                                       : cast<LoadInst>(I)->isVolatile());
  // Type *OrigTy = cast<PointerType>(Addr->getType())->getElementType();
  // const uint32_t TypeSize = DL.getTypeStoreSizeInBits(OrigTy);
  // if (Alignment == 0 || Alignment >= 8 || (Alignment % (TypeSize / 8)) == 0)
  // {
  //   if (IsVolatile)
  //     OnAccessFunc = IsWrite ? TsanVolatileWrite[Idx] :
  //     TsanVolatileRead[Idx];
  //   else
  //     OnAccessFunc = IsWrite ? TsanWrite[Idx] : TsanRead[Idx];
  // } else {
  //   if (IsVolatile)
  //     OnAccessFunc = IsWrite ? TsanUnalignedVolatileWrite[Idx]
  //                            : TsanUnalignedVolatileRead[Idx];
  //   else
  //     OnAccessFunc = IsWrite ? TsanUnalignedWrite[Idx] :
  //     TsanUnalignedRead[Idx];
  // }
  OnAccessFunc = IsWrite ? SSBWrite : SSBRead;
  IRB.CreateCall(OnAccessFunc, IRB.CreatePointerCast(Addr, IRB.getInt8PtrTy()));
  if (IsWrite)
    NumInstrumentedWrites++;
  else
    NumInstrumentedReads++;
  return true;
}

static bool visitor(Function &F, const TargetLibraryInfo &TLI) {
  SoftStoreBuffer SSB;
  return SSB.instrumentFunction(F, TLI);
}

void SoftStoreBuffer::initialize(Module &M) {
  IRBuilder<> IRB(M.getContext());
  AttributeList Attr;
  Attr = Attr.addAttribute(M.getContext(), AttributeList::FunctionIndex,
                           Attribute::NoUnwind);
  SSBWrite = M.getOrInsertFunction("__ssb_write", Attr, IRB.getVoidTy(),
                                   IRB.getInt8PtrTy());
  SSBRead = M.getOrInsertFunction("__ssb_read", Attr, IRB.getVoidTy(),
                                  IRB.getInt8PtrTy());
}

/*
 * Legacy PassManager stuffs
 */
struct SoftStoreBufferLegacy : public FunctionPass {
  static char ID;
  StringRef getPassName() const override;
  void getAnalysisUsage(AnalysisUsage &AU) const override;
  SoftStoreBufferLegacy() : FunctionPass(ID) {}

  bool runOnFunction(Function &F) override {
    auto &TLI = getAnalysis<TargetLibraryInfoWrapperPass>().getTLI(F);
    return visitor(F, TLI);
  }
};

char SoftStoreBufferLegacy::ID = 0;

StringRef SoftStoreBufferLegacy::getPassName() const {
  return "SoftStoreBufferLegacyPass";
}

void SoftStoreBufferLegacy::getAnalysisUsage(AnalysisUsage &AU) const {
  AU.addRequired<TargetLibraryInfoWrapperPass>();
}

static RegisterPass<SoftStoreBufferLegacy>
    X("ssb", "SoftStoreBuffer Pass",
      true, // This pass doesn't modify the CFG => true
      false // This pass is not a pure analysis pass => false
    );

static llvm::RegisterStandardPasses
    Y(llvm::PassManagerBuilder::EP_EarlyAsPossible,
      [](const llvm::PassManagerBuilder &Builder,
         llvm::legacy::PassManagerBase &PM) {
        PM.add(new SoftStoreBufferLegacy());
      });

} // namespace

/*
 * New PassManager stuffs
 */
PreservedAnalyses SoftStoreBufferPass::run(Function &F,
                                           FunctionAnalysisManager &FAM) {
  visitor(F, FAM.getResult<TargetLibraryAnalysis>(F));
  return PreservedAnalyses::all();
}

llvm::PassPluginLibraryInfo getSoftStoreBufferPluginInfo() {
  return {LLVM_PLUGIN_API_VERSION, "SoftStoreBuffer", LLVM_VERSION_STRING,
          [](PassBuilder &PB) {
            PB.registerPipelineParsingCallback(
                [](StringRef Name, FunctionPassManager &FPM,
                   ArrayRef<PassBuilder::PipelineElement>) {
                  if (Name == "ssb") {
                    FPM.addPass(SoftStoreBufferPass());
                    return true;
                  }
                  return false;
                });
          }};
}

extern "C" LLVM_ATTRIBUTE_WEAK ::llvm::PassPluginLibraryInfo
llvmGetPassPluginInfo() {
  return getSoftStoreBufferPluginInfo();
}
