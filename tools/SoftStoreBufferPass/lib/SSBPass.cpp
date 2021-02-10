#include "SSBPass.h"

#include "llvm/Passes/PassBuilder.h"
#include "llvm/Passes/PassPlugin.h"
#include "llvm/Support/raw_ostream.h"

#include "llvm/CodeGen/RegAllocRegistry.h"
#include "llvm/IR/IRBuilder.h"
#include "llvm/IR/LegacyPassManager.h"
#include "llvm/Transforms/IPO/PassManagerBuilder.h"

using namespace llvm;

namespace {

/*
 *Pass Implementation
 */
static bool visitor(Function &F) {
  errs() << F.getName() << "\n";
  IRBuilder<> Builder(&*F.getEntryBlock().getFirstInsertionPt());
  Builder.CreateAlloca(Builder.getInt32Ty(), nullptr, "testtt");
  // for (auto &BB : F) {
  //   IRBuilder<> Builder(&*BB.getFirstNonPHI());
  //   // const auto& ctx = LLVMGetGlobalContext(); // just your LLVMContext
  //   // auto* L = ConstantInt::get(Type::getInt32Ty(ctx), 41);
  //   // auto* R = ConstantInt::get(Type::getInt32Ty(ctx), 42);
  //   // Builder.CreateAdd(L, R, "addtmp");
  //   Builder.CreateAlloca(Builder.getInt32Ty(), nullptr, "a");
  // }
  return true;
}

/*
 * Legacy PassManager stuffs
 */
struct LegacySoftStoreBuffer : public FunctionPass {
  static char ID;
  LegacySoftStoreBuffer() : FunctionPass(ID) {}
  // Main entry point - the name conveys what unit of IR this is to be run on.
  bool runOnFunction(Function &F) override { return visitor(F); }
};

char LegacySoftStoreBuffer::ID = 0;

static RegisterPass<LegacySoftStoreBuffer>
    X("ssb", "SoftStoreBuffer Pass",
      true, // This pass doesn't modify the CFG => true
      false // This pass is not a pure analysis pass => false
    );

static llvm::RegisterStandardPasses
    Y(llvm::PassManagerBuilder::EP_EarlyAsPossible,
      [](const llvm::PassManagerBuilder &Builder,
         llvm::legacy::PassManagerBase &PM) {
        PM.add(new LegacySoftStoreBuffer());
      });
} // namespace

/*
 * New PassManager stuffs
 */
PreservedAnalyses SoftStoreBuffer::run(Function &F,
                                       FunctionAnalysisManager &AM) {
  visitor(F);
  return PreservedAnalyses::all();
}

llvm::PassPluginLibraryInfo getSoftStoreBufferPluginInfo() {
  return {LLVM_PLUGIN_API_VERSION, "SoftStoreBuffer", LLVM_VERSION_STRING,
          [](PassBuilder &PB) {
            PB.registerPipelineParsingCallback(
                [](StringRef Name, FunctionPassManager &FPM,
                   ArrayRef<PassBuilder::PipelineElement>) {
                  if (Name == "ssb") {
                    FPM.addPass(SoftStoreBuffer());
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
