#include "llvm/ADT/ArrayRef.h"
#include "llvm/ADT/Statistic.h"
#include "llvm/Analysis/CaptureTracking.h"
#include "llvm/Analysis/ValueTracking.h"
#include "llvm/CodeGen/RegAllocRegistry.h"
#include "llvm/IR/IRBuilder.h"
#include "llvm/IR/InlineAsm.h"
#include "llvm/IR/LegacyPassManager.h"
#include "llvm/Passes/PassBuilder.h"
#include "llvm/Passes/PassPlugin.h"
#include "llvm/ProfileData/InstrProf.h"
#include "llvm/Support/CommandLine.h"
#include "llvm/Support/raw_ostream.h"
#include "llvm/Transforms/IPO/PassManagerBuilder.h"
#include "llvm/Transforms/Utils/Local.h"

#include "pass/SSBPass.h"
#include "pass/entries.h"

using namespace llvm;

#define DEBUG_TYPE "ssb"

static cl::opt<bool> ClBuileKernel("ssb-kernel",
                                   cl::desc("Build a Linux kernel"),
                                   cl::init(false));

static cl::opt<std::string>
    ClMemoryModel("memorymodel", cl::desc("Memory model being emulated"),
                  cl::init(""));

static cl::opt<std::string>
    ClArchitecture("arch",
                   cl::desc("Architecture on which the target program runs"),
                   cl::init(""));

STATISTIC(NumInstrumentedReads, "Number of instrumented reads");
STATISTIC(NumInstrumentedWrites, "Number of instrumented writes");
STATISTIC(NumInstrumentedFlushes, "Number of instrumented flushed");
STATISTIC(NumAccessesWithBadSize, "Number of accesses with bad size");

namespace {

/*
 *Pass Implementation
 */
struct SoftStoreBuffer {
  bool instrumentFunction(Function &F, const TargetLibraryInfo &TLI);

private:
  void initialize(Module &M);
  bool instrumentLoadOrStore(Instruction *I, const DataLayout &DL);
  bool instrumentFlush(Instruction *I);
  FunctionCallee findCallbackFunction();
  bool addrPointsToConstantData(Value *Addr);
  void chooseInstructionsToInstrument(SmallVectorImpl<Instruction *> &Local,
                                      SmallVectorImpl<Instruction *> &All,
                                      const DataLayout &DL);
  int getMemoryAccessFuncIndex(Value *Addr, const DataLayout &DL);
  bool isMemBarrierOfTargetArch(Instruction *I);
  bool isHardIRQEntryOfTargetArch(Function &F);
  bool isSoftIRQEntryOfTargetArch(Function &F);
  bool isIRQEntryOfTargetArch(Function &F);
  bool isSyscallEntryOfTargetArch(Function &F);
  /* Collected instructions */
  SmallVector<Instruction *, 8> AllLoadsAndStores;
  SmallVector<Instruction *, 8> LocalLoadsAndStores;
  SmallVector<Instruction *, 8> AtomicAccesses;
  SmallVector<Instruction *, 8> MemIntrinCalls;
  SmallVector<Instruction *, 8> MemBarrier;
  /* Callbacks */
  // Accesses sizes are powers of two: 1, 2, 4, 8, 16.
  static const size_t kNumberOfAccessSizes = 5;
  enum MemoryModel { TSO, PSO, kNumberOfMemoryModels };
  MemoryModel TargetMemoryModel;
  FunctionCallee SSBLoad[kNumberOfAccessSizes];
  FunctionCallee SSBStore[kNumberOfAccessSizes];
  FunctionCallee SSBFlush;
  enum Architecture { X86_64, Aarch64, kNumberOfArchitectures };
  Architecture TargetArchitecture;
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

static bool isMemBarrier(InlineAsm *Asm,
                         SmallVector<std::string, 8> BarrierStrs) {
  for (auto BarrierStr : BarrierStrs) {
    if (Asm->getAsmString() == BarrierStr)
      return true;
  }
  return false;
}

bool SoftStoreBuffer::isMemBarrierOfTargetArch(Instruction *I) {
#define _barrier(elems...) SmallVector<std::string, 8>(elems)
  SmallVector<std::string, 8> BarrierStrs[] = {
      _barrier({"lfence", "mfence", "sfence"}),        // x86_64
      _barrier({"dmb ish", "dmb ishst", "dmb ishld"}), // aarch64
  };
#undef _barrier
  if (CallInst *CI = dyn_cast<CallInst>(I)) {
    // Inline asm is expressed as an operand of CallInst
    if (CI->isInlineAsm()) {
      auto *Asm = cast<InlineAsm>(CI->getCalledOperand());
      // NOTE: Checking getContraintString() has ~{memory} is not
      // enough since compiler barriers has the constraint but it does
      // not emit a real memory barrier.
      return Asm->hasSideEffects() &&
             isMemBarrier(Asm, BarrierStrs[TargetArchitecture]);
    }
  }
  return false;
}

static bool isEntry(Function &F, std::string Entries[], int size) {
  for (int i = 0; i < size; i++) {
    if (F.getName() == Entries[i])
      return true;
  }
  return false;
}

bool SoftStoreBuffer::isHardIRQEntryOfTargetArch(Function &F) {
#define _LEN(array) (sizeof(array) / sizeof(array[0]))
  if (TargetArchitecture == X86_64)
    return isEntry(F, IRQEntriesX86_64, _LEN(IRQEntriesX86_64));
  else
    return isEntry(F, IRQEntriesArm64, _LEN(IRQEntriesArm64));
#undef _LEN
}

bool SoftStoreBuffer::isSoftIRQEntryOfTargetArch(Function &F) {
  // As far as I know, the softIRQ entry function resides in the
  // .softirqentry.text section.
  return F.getSection() == ".softirqentry.text";
}

bool SoftStoreBuffer::isIRQEntryOfTargetArch(Function &F) {
  return isHardIRQEntryOfTargetArch(F) || isSoftIRQEntryOfTargetArch(F);
}

bool SoftStoreBuffer::isSyscallEntryOfTargetArch(Function &F) {
  // Even though actual syscall entries are defined by SYSCALL_DEFINEx
  // macros, it is enough to check the common path of syscalls for our
  // purpose.
  if (TargetArchitecture == X86_64)
    return isEntry(F, &SyscallEntryX86_64, 1);
  else
    return isEntry(F, &SyscallEntryArm64, 1);
}

bool SoftStoreBuffer::instrumentFunction(Function &F,
                                         const TargetLibraryInfo &TLI) {
  initialize(*F.getParent());

  bool IRQEntry = isIRQEntryOfTargetArch(F);
  bool SyscallEntry = isSyscallEntryOfTargetArch(F);

  if (!F.hasFnAttribute(Attribute::SoftStoreBuffer) && !IRQEntry &&
      !SyscallEntry)
    return false;

  if (F.getSection() == ".noinstr.text") {
    assert(!IRQEntry && !SyscallEntry);
    return false;
  }

  LLVM_DEBUG(dbgs() << "=== Instrumenting a function " << F.getName()
                    << " ===\n");

  if (IRQEntry || SyscallEntry) {
    instrumentFlush(F.getEntryBlock().getTerminator());
    for (auto &BB : F) {
      for (auto &I : BB)
        if (isa<ReturnInst>(I))
          instrumentFlush(BB.getFirstNonPHI());
    }
    // We do not instrument other instructions in IRQ entry functions.
    if (IRQEntry)
      return true;
  }

  bool Res = false;
  bool HasCalls = false;
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
        if (isMemBarrierOfTargetArch(&Inst))
          MemBarrier.push_back(&Inst);
        HasCalls = true;
        chooseInstructionsToInstrument(LocalLoadsAndStores, AllLoadsAndStores,
                                       DL);
      }
    }
    chooseInstructionsToInstrument(LocalLoadsAndStores, AllLoadsAndStores, DL);
  }

  // We have collected all loads and stores.
  for (auto Inst : AllLoadsAndStores)
    Res |= instrumentLoadOrStore(Inst, DL);

  for (auto Inst : MemBarrier)
    Res |= instrumentFlush(Inst);

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

  int Idx = getMemoryAccessFuncIndex(Addr, DL);
  if (Idx < 0)
    return false;

  LLVM_DEBUG(dbgs() << "Instrumenting a " << (IsWrite ? "store" : "load")
                    << " callback at " << *I << "\n");

  // TODO: Alignment / Volatile
  // Ref:toolchains/llvm/llvm/lib/Transforms/Instrumentation/ThreadSanitizer.cpp#597
  OnAccessFunc = IsWrite ? SSBStore[Idx] : SSBLoad[Idx];
  auto Args = SmallVector<Value *, 8>();
  Args.push_back(IRB.CreatePointerCast(Addr, IRB.getInt8PtrTy()));
  if (IsWrite) {
    // Store requires one more argument
    Args.push_back(
        IRB.CreatePointerCast(I->getOperand(0) /* == SI->getValueOperand() */,
                              IRB.getIntNTy((1U << Idx) * 8)));
  }
  auto *CI = IRB.CreateCall(OnAccessFunc, Args);
  // A call to a callback function is instrumented
  if (!IsWrite)
    // Now we may need to replace Uses of LoadInst with CI.
    I->replaceAllUsesWith(IRB.CreateIntToPtr(CI, I->getType()));
  // We replaced all I's Uses so we can remove it.
  I->eraseFromParent();
  if (IsWrite)
    NumInstrumentedWrites++;
  else
    NumInstrumentedReads++;
  return true;
}

bool SoftStoreBuffer::instrumentFlush(Instruction *I) {
  IRBuilder<> IRB(I);
  LLVM_DEBUG(dbgs() << "Instrumenting a membarrier callback at " << *I << "\n");
  IRB.CreateCall(SSBFlush, ConstantPointerNull::get(IRB.getInt8PtrTy()));
  NumInstrumentedFlushes++;
  return true;
}

static bool visitor(Function &F, const TargetLibraryInfo &TLI) {
  SoftStoreBuffer SSB;
  return SSB.instrumentFunction(F, TLI);
}

void SoftStoreBuffer::initialize(Module &M) {
  TargetMemoryModel = ClMemoryModel == "TSO" ? TSO : PSO;
  TargetArchitecture = ClArchitecture == "x86_64" ? X86_64 : Aarch64;
  IRBuilder<> IRB(M.getContext());
  AttributeList Attr;
  Attr = Attr.addAttribute(M.getContext(), AttributeList::FunctionIndex,
                           Attribute::NoUnwind);
  std::string TargetMemoryModelStr = (TargetMemoryModel == TSO) ? "tso" : "pso";
  for (size_t i = 0; i < kNumberOfAccessSizes; i++) {
    const unsigned ByteSize = 1U << i;
    const unsigned BitSize = ByteSize * 8;
    std::string ByteSizeStr = utostr(ByteSize);
    std::string BitSizeStr = utostr(BitSize);
    Type *IntNTy = IRB.getIntNTy(BitSize);
    SmallString<32> StoreName("__ssb_" + TargetMemoryModelStr + "_store" +
                              ByteSizeStr);
    SSBStore[i] = M.getOrInsertFunction(StoreName, Attr, IRB.getVoidTy(),
                                        IRB.getInt8PtrTy(), IntNTy);
    SmallString<32> LoadName("__ssb_" + TargetMemoryModelStr + "_load" +
                             ByteSizeStr);
    SSBLoad[i] =
        M.getOrInsertFunction(LoadName, Attr, IntNTy, IRB.getInt8PtrTy());
    SmallString<32> FlushName("__ssb_" + TargetMemoryModelStr + "_flush");
    SSBFlush = M.getOrInsertFunction(FlushName, Attr, IRB.getVoidTy(),
                                     IRB.getInt8PtrTy());
  }
}

int SoftStoreBuffer::getMemoryAccessFuncIndex(Value *Addr,
                                              const DataLayout &DL) {
  Type *OrigPtrTy = Addr->getType();
  Type *OrigTy = cast<PointerType>(OrigPtrTy)->getElementType();
  assert(OrigTy->isSized());
  uint32_t TypeSize = DL.getTypeStoreSizeInBits(OrigTy);
  if (TypeSize != 8 && TypeSize != 16 && TypeSize != 32 && TypeSize != 64 &&
      TypeSize != 128) {
    NumAccessesWithBadSize++;
    // Ignore all unusual sizes.
    return -1;
  }
  size_t Idx = countTrailingZeros(TypeSize / 8);
  assert(Idx < kNumberOfAccessSizes);
  return Idx;
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
