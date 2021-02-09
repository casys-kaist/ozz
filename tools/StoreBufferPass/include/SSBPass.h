#ifndef __SOFT_STORE_BUFFER_H
#define __SOFT_STORE_BUFFER_H

#include "llvm/IR/PassManager.h"

namespace llvm {

class SoftStoreBuffer : public PassInfoMixin<SoftStoreBuffer> {
public:
  PreservedAnalyses run(Function &F, FunctionAnalysisManager &AM);
};

} // namespace llvm

#endif // __SOFT_STORE_BUFFER_H
