#include "llvm/ADT/ArrayRef.h"
#include "llvm/ADT/Statistic.h"
#include "llvm/Analysis/CaptureTracking.h"
#include "llvm/Analysis/ValueTracking.h"
#include "llvm/CodeGen/RegAllocRegistry.h"
#include "llvm/IR/IRBuilder.h"
#include "llvm/IR/InlineAsm.h"
#include "llvm/IR/LegacyPassManager.h"
#include "llvm/IR/MDBuilder.h"
#include "llvm/Passes/PassBuilder.h"
#include "llvm/Passes/PassPlugin.h"
#include "llvm/ProfileData/InstrProf.h"
#include "llvm/Support/CommandLine.h"
#include "llvm/Support/raw_ostream.h"
#include "llvm/Transforms/IPO/PassManagerBuilder.h"
#include "llvm/Transforms/Utils/BasicBlockUtils.h"
#include "llvm/Transforms/Utils/Local.h"

#include "pass/SSBPass.h"
#include "pass/entries.h"
#include "llvm/ADT/DenseSet.h"

using namespace llvm;

#define DEBUG_TYPE "ssb"

static cl::opt<bool> ClDumpIRs(
    "dump-ir",
    cl::desc(
        "Dump IRs before and after instrumenting callbacks (for debugging)"),
    cl::init(false));

static cl::opt<bool> ClInstrumentOutofScopeCalls(
    "instrument-out-of-scope",
    cl::desc(
        "Instrument the flush callback before calling out-of-scope functions"),
    cl::init(true));

static cl::opt<bool>
    ClFlushEntryOnly("ssb-flush-only",
                     cl::desc("Only instrument the flush callback at entry "
                              "functions of IRQs and syscalls"),
                     cl::init(false));

static cl::opt<bool> ClInstrumentFlush("instrument-flush",
                                       cl::desc("instrument flush callbacks"),
                                       cl::init(true));

static cl::opt<std::string>
    ClMemoryModel("memorymodel", cl::desc("Memory model being emulated"),
                  cl::init(""));

static cl::opt<std::string>
    ClArchitecture("arch",
                   cl::desc("Architecture on which the target program runs"),
                   cl::init(""));

static cl::opt<std::string> ClFuncListFileName(
    "ssb-function-list-filename",
    cl::desc("File name containing a list of to-be-instrumented functions. If "
             "it is a relative "
             "path, it starts from $PROJECT_HOME/tmp"),
    cl::init("to-be-instrumented-functions.lst"));

static cl::opt<bool> ClSecondPass(
    "ssb-second-pass",
    cl::desc(
        "true if it is the second pass. In the first pass, to-be-instrumented "
        "functions are collected into instrumented-function-list. The second "
        "pass is intended to instrument the binary"),
    cl::init(true));

STATISTIC(NumInstrumentedReads, "Number of instrumented reads");
STATISTIC(NumInstrumentedWrites, "Number of instrumented writes");
STATISTIC(NumInstrumentedFlushes, "Number of instrumented flushed");
STATISTIC(NumInstrumentedRetCheck, "Number of instrumented return check");
STATISTIC(NumInstrumentedFuncEntry, "Number of instrumented function entry");
STATISTIC(NumAccessesWithBadSize, "Number of accesses with bad size");

namespace {

static BasicBlock::iterator getFirstNonPHIOrDbgOrAlloca(BasicBlock *bb);

static std::string getIFLFileName() {
  // TODO: Clarify lifetimes of string variations (i.e., StringRef,
  // SmallString, std:: string). And then clean this function.
  // TODO: Seperate the file into multiple files if necessary.
  const char *env_p = std::getenv("PROJECT_HOME");
  StringRef EnvRef = StringRef(env_p);
#define MAXLEN 256
  SmallString<MAXLEN> FileName(EnvRef);
  FileName += "/tmp/";
  FileName += ClFuncListFileName;
  return std::string(FileName);
}

typedef DenseSet<StringRef> InstrumentedFunctionList;

struct InstrumentedFunctionListPass : public ModulePass {
  static char ID;
  InstrumentedFunctionList ifl;

  InstrumentedFunctionListPass() : ModulePass(ID) {}
  ~InstrumentedFunctionListPass();
  StringRef getPassName() const override;
  InstrumentedFunctionList &getIFL() { return ifl; }
  bool runOnModule(Module &M) override;
  void getAnalysisUsage(AnalysisUsage &AU) const override;
};

char InstrumentedFunctionListPass::ID = 0;

StringRef InstrumentedFunctionListPass::getPassName() const {
  return "InstrumentedFunctionListPass";
}

InstrumentedFunctionListPass::~InstrumentedFunctionListPass() {
  for (auto it = ifl.begin(); it != ifl.end(); ++it) {
    StringRef S = *it;
    delete S.data();
  }
}

// TODO: Oh no...
#include <stdio.h>
bool InstrumentedFunctionListPass::runOnModule(Module &M) {
  if (!ClSecondPass)
    return false;

  std::error_code EC;
  std::string fn = getIFLFileName();

  LLVM_DEBUG(dbgs() << "Reading " << fn << "\n");

  // TODO: Does LLVM not provide istream? What the heck is this...
  FILE *fp = fopen(fn.c_str(), "r");
  if (fp == NULL)
    return false;

  char *line = NULL;
  size_t len = 0;
  ssize_t size;

  while ((size = getline(&line, &len, fp)) != -1) {
    assert(size > 0);
    // Cut the delimiter first
    line[size - 1] = 0;
    char *buf = new char[size];
    strncpy(buf, line, size);
    StringRef s(buf);
    // LLVM_DEBUG(dbgs() << s << "\n");
    ifl.insert(s);
  }
  free(line);
  fclose(fp);

  return false;
}

void InstrumentedFunctionListPass::getAnalysisUsage(AnalysisUsage &AU) const {
  AU.setPreservesAll();
}

static RegisterPass<InstrumentedFunctionListPass>
    XX("tfl", "Summarize to-be-instrumented functions", true, true);

static llvm::RegisterStandardPasses
    YY(llvm::PassManagerBuilder::EP_EarlyAsPossible,
       [](const llvm::PassManagerBuilder &Builder,
          llvm::legacy::PassManagerBase &PM) {
         PM.add(new InstrumentedFunctionListPass());
       });

/*
 *Pass Implementation
 */
struct SoftStoreBuffer {
  bool instrumentFunction(Function &F, const TargetLibraryInfo &TLI,
                          const InstrumentedFunctionList &IFL);

private:
  void initialize(Module &M);
  bool instrumentFlushOnly(Function &F, bool DoInstrument);
  bool instrumentAll(Function &F, const TargetLibraryInfo &TLI,
                     const InstrumentedFunctionList &IFL);
  bool instrumentLoadOrStore(Instruction *I, const DataLayout &DL);
  bool instrumentFlush(Instruction *I);
  bool instrumentRetCheck(Instruction *I);
  bool instrumentFuncEntry(Instruction *I);
  FunctionCallee findCallbackFunction();
  bool addrPointsToConstantData(Value *Addr);
  void chooseInstructionsToInstrument(SmallVectorImpl<Instruction *> &Local,
                                      SmallVectorImpl<Instruction *> &All,
                                      const DataLayout &DL);
  int getMemoryAccessFuncIndex(Value *Addr, const DataLayout &DL);
  bool isInterestingLoadStore(Instruction *I);
  bool isMemBarrierOfTargetArch(Instruction *I);
  bool isBUG(Instruction *I);
  bool isOutofScopeCall(Instruction *I, const InstrumentedFunctionList &IFL);
  bool isHardIRQEntryOfTargetArch(Function &F);
  bool isSoftIRQEntryOfTargetArch(Function &F);
  bool isIRQEntryOfTargetArch(Function &F);
  bool isSyscallEntryOfTargetArch(Function &F);
  BasicBlock *SSBDoEmulateHelper(Instruction *I);
  void instrumentHelper(Instruction *I, FunctionCallee callback);
  void SetNoSanitizeMetadata(Instruction *I) {
    I->setMetadata(I->getModule()->getMDKindID("nosanitize"),
                   MDNode::get(I->getContext(), None));
  }
  /* Collected instructions */
  SmallVector<Instruction *, 8> AllLoadsAndStores;
  SmallVector<Instruction *, 8> LocalLoadsAndStores;
  SmallVector<Instruction *, 8> AtomicAccesses;
  SmallVector<Instruction *, 8> MemIntrinCalls;
  SmallVector<Instruction *, 8> MemBarrier;
  SmallVector<Instruction *, 8> AllReturns;
  SmallVector<Instruction *, 8> OutofScopeCalls;
  /* Callbacks */
  // Accesses sizes are powers of two: 1, 2, 4, 8, 16.
  static const size_t kNumberOfAccessSizes = 5;
  enum MemoryModel { TSO, PSO, kNumberOfMemoryModels };
  MemoryModel TargetMemoryModel;
  FunctionCallee SSBLoad[kNumberOfAccessSizes];
  FunctionCallee SSBStore[kNumberOfAccessSizes];
  FunctionCallee SSBFlush;
  FunctionCallee SSBRetCheck;
  FunctionCallee SSBFuncEntry;
  Constant *SSBDoEmulate;
  enum Architecture { X86_64, Aarch64, kNumberOfArchitectures };
  Architecture TargetArchitecture;
  void appendFunctionName(Function &F);
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

BasicBlock *SoftStoreBuffer::SSBDoEmulateHelper(Instruction *I) {
  IRBuilder<> IRB(I);
  auto *DoEmulate = IRB.CreateLoad(SSBDoEmulate);
  SetNoSanitizeMetadata(DoEmulate);
  // If __do_emulate == 1
  Value *CmpInst = IRB.CreateICmpEQ(DoEmulate, IRB.getInt8(1));
  MDBuilder MDB(I->getContext());
  MDNode *BranchWeights =
      MDB.createBranchWeights(1 /*ThenBlock*/, 10 /*ElseBlock*/);
  Instruction *CheckTerm =
      SplitBlockAndInsertIfThen(CmpInst, I, false, BranchWeights);
  BasicBlock *ThenBlock = CheckTerm->getParent();
  // ThenBlock -- slowpath (store buffer emulation)
  return ThenBlock;
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
    if (isa<AllocaInst>(getUnderlyingObject(Addr)) &&
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

static bool isRMWBitOps(Instruction *I) {
  std::string RMWBitOpsFunc[] = {"test_and_set_bit", "test_and_clear_bit",
                                 "test_and_change_bit",
                                 "test_and_set_bit_lock"};
  if (CallInst *CI = dyn_cast<CallInst>(I)) {
    auto *F = CI->getCalledFunction();
    if (!F)
      return false;
    for (auto f : RMWBitOpsFunc) {
      if (F->getName().startswith(f))
        return true;
    }
  }
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

bool SoftStoreBuffer::isInterestingLoadStore(Instruction *I) {
  if (auto *LI = dyn_cast<LoadInst>(I))
    return !LI->isAtomic() && LI->getSyncScopeID() != SyncScope::SingleThread;
  else if (auto *SI = dyn_cast<StoreInst>(I))
    return !SI->isAtomic() && SI->getSyncScopeID() != SyncScope::SingleThread;
  else
    return false;
}

bool SoftStoreBuffer::instrumentFlushOnly(Function &F, bool DoInstrument) {
  LLVM_DEBUG(dbgs() << "=== Instrumenting a function (flush-only)"
                    << F.getName() << " ===\n");
  if (!DoInstrument)
    return false;

  instrumentFlush(F.getEntryBlock().getTerminator());
  SmallVector<Instruction *, 8> NeedInstrument;
  NeedInstrument.push_back(F.getEntryBlock().getTerminator());
  for (auto &BB : F) {
    for (auto &I : BB)
      if (isa<ReturnInst>(I))
        NeedInstrument.push_back(BB.getFirstNonPHI());
  }
  for (auto Inst : NeedInstrument)
    instrumentFlush(Inst);
  // We do not instrument other instructions in entry functions.
  return true;
}

static bool isBUG_X86_64(Instruction *I) {
  if (CallInst *CI = dyn_cast<CallInst>(I)) {
    if (CI->isInlineAsm()) {
      auto *Asm = cast<InlineAsm>(CI->getCalledOperand());
      auto Str = Asm->getAsmString();
#define UD2 ".byte 0x0f, 0x0"
      return Str.find(UD2) != std::string::npos;
    }
  }
  return false;
}

bool SoftStoreBuffer::isBUG(Instruction *I) {
  if (TargetArchitecture == X86_64) {
    return isBUG_X86_64(I);
  } else {
    // TODO: aarch64
    return false;
  }
}

bool SoftStoreBuffer::instrumentAll(Function &F, const TargetLibraryInfo &TLI,
                                    const InstrumentedFunctionList &IFL) {
  LLVM_DEBUG(dbgs() << "=== Instrumenting a function " << F.getName()
                    << " ===\n");

  // Early checks
  if (F.hasFnAttribute(Attribute::NoSoftStoreBuffer))
    return false;

  if (F.getSection() == ".noinstr.text")
    return false;

  if (!ClSecondPass) {
    appendFunctionName(F);
    return false;
  }

  // Now we are instrumenting callbacks
  bool Res = false;
  bool HasCalls = false;
  const DataLayout &DL = F.getParent()->getDataLayout();

  // Visiting and cheking all instructions
  for (auto &BB : F) {
    for (auto &Inst : BB) {
      if (isAtomic(&Inst) || isRMWBitOps(&Inst))
        AtomicAccesses.push_back(&Inst);
      else if (isInterestingLoadStore(&Inst))
        LocalLoadsAndStores.push_back(&Inst);
      else if (isa<ReturnInst>(&Inst))
        AllReturns.push_back(&Inst);
      else if (isa<CallInst>(Inst) || isa<InvokeInst>(Inst)) {
        if (CallInst *CI = dyn_cast<CallInst>(&Inst))
          maybeMarkSanitizerLibraryCallNoBuiltin(CI, &TLI);
        if (isOutofScopeCall(&Inst, IFL))
          OutofScopeCalls.push_back(&Inst);
        if (isa<MemIntrinsic>(Inst))
          MemIntrinCalls.push_back(&Inst);
        if (isMemBarrierOfTargetArch(&Inst))
          MemBarrier.push_back(&Inst);
        HasCalls = true;
        chooseInstructionsToInstrument(LocalLoadsAndStores, AllLoadsAndStores,
                                       DL);
        if (isBUG(&Inst))
          break;
      }
    }
    chooseInstructionsToInstrument(LocalLoadsAndStores, AllLoadsAndStores, DL);
  }

  // We have collected all loads and stores.
  for (auto Inst : AllLoadsAndStores)
    Res |= instrumentLoadOrStore(Inst, DL);

  for (auto Inst : MemBarrier)
    Res |= instrumentFlush(Inst);

  for (auto Inst : AtomicAccesses)
    Res |= instrumentFlush(Inst);

  if (ClInstrumentOutofScopeCalls)
    for (auto Inst : OutofScopeCalls)
      Res |= instrumentFlush(Inst);

  // Function entry callbacks
  auto &entryBB = F.getEntryBlock();
  auto *firstInst = &*getFirstNonPHIOrDbgOrAlloca(&entryBB);
  instrumentFuncEntry(firstInst);

  if (F.getName() != "pso_test_breakpoint")
    // TODO: As our return check callback is incomplete, it flushes
    // the store buffer when returning from pso_test_breakpoint()
    // preventing the integration test. This function is only for the
    // integration testing so it is totally fine not to instrument the
    // callback. Remove this if statement after completing the return
    // check mechanism.
    for (auto Inst : AllReturns)
      instrumentRetCheck(Inst);

  return Res | HasCalls;
}

bool SoftStoreBuffer::instrumentFunction(Function &F,
                                         const TargetLibraryInfo &TLI,
                                         const InstrumentedFunctionList &IFL) {
  initialize(*F.getParent());

  bool IRQEntry = isIRQEntryOfTargetArch(F);
  bool SyscallEntry = isSyscallEntryOfTargetArch(F);

  if (IRQEntry || SyscallEntry || ClFlushEntryOnly)
    return instrumentFlushOnly(F, IRQEntry || SyscallEntry);
  else
    return instrumentAll(F, TLI, IFL);
}

bool SoftStoreBuffer::instrumentLoadOrStore(Instruction *I,
                                            const DataLayout &DL) {
  IRBuilder<> IRB(I);
  bool IsWrite = isa<StoreInst>(*I);
  Value *Addr = IsWrite ? cast<StoreInst>(I)->getPointerOperand()
                        : cast<LoadInst>(I)->getPointerOperand();
  FunctionCallee OnAccessFunc = nullptr;
  Type *Ty = I->getType();

  // swifterror memory addresses are mem2reg promoted by instruction
  // selection. As such they cannot have regular uses like an instrumentation
  // function and it makes no sense to track them as memory.
  if (Addr->isSwiftError())
    return false;

  int Idx = getMemoryAccessFuncIndex(Addr, DL);
  if (Idx < 0)
    return false;
  OnAccessFunc = IsWrite ? SSBStore[Idx] : SSBLoad[Idx];

  LLVM_DEBUG(dbgs() << "Instrumenting a " << (IsWrite ? "store" : "load")
                    << " callback at " << *I << "\n");

  if (IsWrite)
    NumInstrumentedWrites++;
  else
    NumInstrumentedReads++;

  // Check we can use the fastpath
  auto *DoEmulate = IRB.CreateLoad(SSBDoEmulate);
  SetNoSanitizeMetadata(DoEmulate);

  // If __do_emulate != 1
  Value *CmpInst = IRB.CreateICmpNE(DoEmulate, IRB.getInt8(1));
  Instruction *ThenTerm, *ElseTerm;
  MDBuilder MDB(I->getContext());
  MDNode *BranchWeights =
      MDB.createBranchWeights(10 /*ThenBlock*/, 1 /*ElseBlock*/);
  SplitBlockAndInsertIfThenElse(CmpInst, I, &ThenTerm, &ElseTerm,
                                BranchWeights);

  // ThenBlock -- fastpath
  BasicBlock *NewTail = I->getParent();
  I->removeFromParent();
  IRBuilder<> IRBThen(ThenTerm);
  IRBThen.Insert(I);

  // ElseBlock -- slowpath (store buffer emulation)
  IRBuilder<> IRBElse(ElseTerm);
  auto Args = SmallVector<Value *, 8>();
  Args.push_back(IRBElse.CreatePointerCast(Addr, IRBElse.getInt8PtrTy()));
  if (IsWrite) {
    // Store requires one more argument
    Args.push_back(IRBElse.CreatePointerCast(
        I->getOperand(0) /* == SI->getValueOperand() */,
        IRBElse.getIntNTy((1U << Idx) * 8)));
  }
  auto *CI = IRBElse.CreateCall(OnAccessFunc, Args);
  auto *Res = IRBElse.CreateIntToPtr(CI, Ty);

  // PHI instruction to select the result
  if (!IsWrite) {
    IRBuilder<> IRBTail(NewTail->getFirstNonPHI());
    auto *phi = IRBTail.CreatePHI(Ty, 2);
    phi->addIncoming(I, ThenTerm->getParent());
    phi->addIncoming(Res, ElseTerm->getParent());
    I->replaceUsesWithIf(phi, [phi](Use &U) {
      auto *I = U.getUser();
      // Don't replace if it's an instruction in the BB basic block.
      return I != phi;
    });
  }

  return true;
}

bool SoftStoreBuffer::instrumentFlush(Instruction *I) {
  if (!ClInstrumentFlush)
    return false;

  LLVM_DEBUG(dbgs() << "Instrumenting a membarrier callback at " << *I << "\n");
  NumInstrumentedFlushes++;

  BasicBlock *ThenBlock = SSBDoEmulateHelper(I);
  // ThenBlock -- slowpath (store buffer emulation)
  IRBuilder<> IRBThen(ThenBlock->getFirstNonPHI());
  IRBThen.CreateCall(SSBFlush);
  return true;
}

void SoftStoreBuffer::instrumentHelper(Instruction *I,
                                       FunctionCallee callback) {
  BasicBlock *ThenBlock = SSBDoEmulateHelper(I);
  IRBuilder<> IRBThen(ThenBlock->getFirstNonPHI());

  Value *ReturnAddress = IRBThen.CreateCall(
      Intrinsic::getDeclaration(I->getModule(), Intrinsic::returnaddress),
      IRBThen.getInt32(0));

  auto Args = SmallVector<Value *, 8>();
  Args.push_back(ReturnAddress);
  IRBThen.CreateCall(callback, Args);
}

bool SoftStoreBuffer::instrumentRetCheck(Instruction *I) {
  if (!ClInstrumentFlush)
    // Retchk is also a kind of flush callback
    return false;
  LLVM_DEBUG(dbgs() << "Instrumenting a retchk callback at " << *I << "\n");
  NumInstrumentedRetCheck++;
  instrumentHelper(I, SSBRetCheck);
  return true;
}

bool SoftStoreBuffer::instrumentFuncEntry(Instruction *I) {
  if (!ClInstrumentFlush)
    // Entry callbacks are for helping retchk callbacks. So if we do
    // not retchk, then entry callbacks are pointless
    return false;
  LLVM_DEBUG(dbgs() << "Instrumenting an function-entry callback at " << *I
                    << "\n");
  NumInstrumentedFuncEntry++;
  instrumentHelper(I, SSBFuncEntry);
  return true;
}

static void dumpIR(Function &F, std::string prefix) {
  const char *tmpdirp;
  std::string tmpdir;
  if ((tmpdirp = std::getenv("TMP_DIR")))
    tmpdir.append(tmpdirp);

  std::string fn = tmpdir + "/" + F.getName().str() + "." + prefix + ".ll";
  std::error_code EC;

  raw_fd_ostream out(fn, EC, sys::fs::OF_Text);

  F.print(out, NULL /*default*/, false /*default*/, true /*IsForDebug*/);
}

static bool visitor(Function &F, const TargetLibraryInfo &TLI,
                    const InstrumentedFunctionList &IFL) {
  SoftStoreBuffer SSB;
  bool ret;
  if (ClDumpIRs)
    dumpIR(F, std::string("before"));
  ret = SSB.instrumentFunction(F, TLI, IFL);
  if (ClDumpIRs)
    dumpIR(F, std::string("after"));
  return ret;
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
    SSBFlush = M.getOrInsertFunction(FlushName, Attr, IRB.getVoidTy());
    SmallString<32> RetCheckName("__ssb_" + TargetMemoryModelStr + "_retchk");
    SSBRetCheck = M.getOrInsertFunction(RetCheckName, Attr, IRB.getVoidTy(),
                                        IRB.getInt8PtrTy());
    SmallString<32> FuncEntryName("__ssb_" + TargetMemoryModelStr +
                                  "_funcentry");
    SSBFuncEntry = M.getOrInsertFunction(FuncEntryName, Attr, IRB.getVoidTy(),
                                         IRB.getInt8PtrTy());
    SSBDoEmulate = M.getOrInsertGlobal("__ssb_do_emulate", IRB.getInt8Ty());
  }
}

int SoftStoreBuffer::getMemoryAccessFuncIndex(Value *Addr,
                                              const DataLayout &DL) {
  Type *OrigPtrTy = Addr->getType();
  Type *OrigTy = cast<PointerType>(OrigPtrTy)->getElementType();
  assert(OrigTy->isSized());
  uint32_t TypeSize = DL.getTypeStoreSizeInBits(OrigTy);
  if (TypeSize != 8 && TypeSize != 16 && TypeSize != 32 && TypeSize != 64) {
    NumAccessesWithBadSize++;
    // Ignore all unusual sizes.
    return -1;
  }
  size_t Idx = countTrailingZeros(TypeSize / 8);
  assert(Idx < kNumberOfAccessSizes);
  return Idx;
}

void SoftStoreBuffer::appendFunctionName(Function &F) {
  StringRef fn = F.getName();
  // NOTE: We treat weak symbols as not-instrumented since its
  // corresponding strong symbol may not be instrumented.
  // TODO: Determine if the strong symbol is really not instrumented.
  if (F.hasWeakLinkage())
    return;
  std::error_code EC;
  LLVM_DEBUG(dbgs() << "Writing " << fn << "\n");
  raw_fd_ostream out(getIFLFileName(), EC, sys::fs::OF_Append);
  if (!EC)
    out << fn << '\n';
  else
    LLVM_DEBUG(dbgs() << "error opening file for writing");
  out.close();
}

static bool isEntryBlock(BasicBlock *bb) {
  const Function *F = bb->getParent();
  assert(F && "Block must have a parent function to use this API");
  return bb == &F->getEntryBlock();
}

// NOTE: Copied from a recent version of LLVM
static BasicBlock::iterator getFirstNonPHIOrDbgOrAlloca(BasicBlock *bb) {
  Instruction *FirstNonPHI = bb->getFirstNonPHI();
  if (!FirstNonPHI)
    return bb->end();

  BasicBlock::iterator InsertPt = FirstNonPHI->getIterator();
  if (InsertPt->isEHPad())
    ++InsertPt;

  if (isEntryBlock(bb)) {
    BasicBlock::const_iterator End = bb->end();
    while (InsertPt != End &&
           (isa<AllocaInst>(*InsertPt) || isa<DbgInfoIntrinsic>(*InsertPt) ||
            isa<PseudoProbeInst>(*InsertPt))) {
      if (const AllocaInst *AI = dyn_cast<AllocaInst>(&*InsertPt)) {
        if (!AI->isStaticAlloca())
          break;
      }
      ++InsertPt;
    }
  }
  return InsertPt;
}

static bool isIndirectCall(CallBase *CB) { return CB->isIndirectCall(); }

static bool isNotInstrumentedCall(CallBase *CB,
                                  const InstrumentedFunctionList &IFL) {
  auto *F = CB->getCalledFunction();
  // XXX: This check is possibly redundant with isIndirectCall().
  return (F ? IFL.find(F->getName()) == IFL.end() : true);
}

static bool isAssumeLikeIntrinsic(IntrinsicInst *II) {
  // NOTE: LLVM 13.0.0 provides this method as a member function of
  // IntrinsicInst. As we are using LLVM 11.0.0, just copy-and-paste
  // it.
  // Ref:
  // https://llvm.org/doxygen/classllvm_1_1IntrinsicInst.html#a00e7e0d4898946398f1c351251b8c7d2
  switch (II->getIntrinsicID()) {
  default:
    break;
  case Intrinsic::assume:
  case Intrinsic::sideeffect:
  // case Intrinsic::pseudoprobe:
  case Intrinsic::dbg_declare:
  case Intrinsic::dbg_value:
  case Intrinsic::dbg_label:
  case Intrinsic::invariant_start:
  case Intrinsic::invariant_end:
  case Intrinsic::lifetime_start:
  case Intrinsic::lifetime_end:
  // case Intrinsic::experimental_noalias_scope_decl:
  case Intrinsic::objectsize:
  case Intrinsic::ptr_annotation:
  case Intrinsic::var_annotation:
    return true;
  }
  return false;
}

static bool is__kasan_check_read_write(StringRef name) {
  return name.startswith("__kasan_check_read") ||
         name.startswith("__kasan_check_write");
}

static bool isCallingAnnotatedInlineAsm(CallBase *CB) {
#define NO_BARRIER_SEMANTIC "no kssb"
  if (CB->isInlineAsm()) {
    auto *Asm = cast<InlineAsm>(CB->getCalledOperand());
    const std::string &AsmString = Asm->getAsmString();
    bool annotated = AsmString.find(NO_BARRIER_SEMANTIC) != std::string::npos ||
                     AsmString.length() == 0;
    LLVM_DEBUG(dbgs() << "inline asm: " << AsmString << "\n");
    LLVM_DEBUG(dbgs() << "annotated: " << annotated << "\n");
    return annotated;
  }
  return false;
}

bool SoftStoreBuffer::isOutofScopeCall(Instruction *I,
                                       const InstrumentedFunctionList &IFL) {
  assert(isa<CallBase>(I));
  auto *CB = cast<CallBase>(I);
  if (IFL.empty()) {
    // XXX: We don't have a list of target functions so we cannot
    // determine the CB's callee is out-of-scope or not. As a
    // workaround, always return false. This will probably make the
    // kernel not bootable.
    return false;
  }

  if (auto *II = dyn_cast<IntrinsicInst>(I))
    // Intrinsic function calls are sometimes used to annotate
    // semantics, and do not generate any real code. We don't need to
    // instrument the flush callback in this case.
    return !isAssumeLikeIntrinsic(II);

  auto *F = CB->getCalledFunction();
  if (F && is__kasan_check_read_write(F->getName())) {
    // __SANITIZE_ADDRESS__ is defined when building Linux with clang,
    // and accordingly, __kasan_check_{read, write} is called during
    // the runtime. As we don't want to flush the store buffer when
    // KASAN callbacks are called, do not treat the callbacks as
    // out-of-scope-calls.
    return false;
  }

  if (isCallingAnnotatedInlineAsm(CB)) {
    // We annotate inline assemblies that does not surely have memory
    // barrier semantics. We don't need to instrument the flush
    // callback before those inline assemblies.
    return false;
  }

  bool ret = isIndirectCall(CB) || isNotInstrumentedCall(CB, IFL);
  if (ret)
    LLVM_DEBUG(dbgs() << *I << " is calling a function out-of-scope\n");
  return ret;
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
    auto &IFL = getAnalysis<InstrumentedFunctionListPass>().getIFL();
    return visitor(F, TLI, IFL);
  }
};

char SoftStoreBufferLegacy::ID = 0;

StringRef SoftStoreBufferLegacy::getPassName() const {
  return "SoftStoreBufferLegacyPass";
}

void SoftStoreBufferLegacy::getAnalysisUsage(AnalysisUsage &AU) const {
  AU.addRequired<TargetLibraryInfoWrapperPass>();
  AU.addRequired<InstrumentedFunctionListPass>();
}

static RegisterPass<SoftStoreBufferLegacy>
    X("ssb", "SoftStoreBuffer Pass",
      true, // This pass doesn't modify the CFG => true
      false // This pass is not a pure analysis pass => false
    );

static llvm::RegisterStandardPasses
    Y(llvm::PassManagerBuilder::EP_OptimizerLast,
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
  // TODO: We are not using the new pass manager stuff. Implement this
  // later.
  // visitor(F, FAM.getResult<TargetLibraryAnalysis>(F));
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
