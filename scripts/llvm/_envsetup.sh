#!/bin/sh -e

__export_envvar "LLVM" "$TOOLCHAINS_DIR/llvm"
__append_path "$LLVM_INSTALL/bin"
export CLANG="$LLVM_INSTALL/bin/clang"
export LLVM_VERSION="llvmorg-11.0.1"
