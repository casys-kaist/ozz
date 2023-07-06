#!/bin/sh -e

__export_envvar "LLVM" "$TOOLCHAINS_DIR/llvm"
__append_path "$LLVM_INSTALL/bin"
export LD_LIBRARY_PATH="$LLVM_INSTALL/lib"${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}
export CLANG="$LLVM_INSTALL/bin/clang"
export LLVM_VERSION="llvmorg-12.0.1"
