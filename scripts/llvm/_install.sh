#!/bin/sh -e

[ -n "$__RELRAZZER_READY" ] || exit 1

. "$SCRIPTS_DIR/llvm/_envsetup.sh"

_download() {
	REPO="https://github.com/llvm/llvm-project.git"
	__git_clone $REPO $LLVM_PATH $LLVM_VERSION
}

_build() {
	_ENABLE="-DLLVM_ENABLE_PROJECTS=clang;compiler-rt"
	__make_dir_and_exec_cmd "$LLVM_BUILD" \
							"cmake -G 'Ninja' $_ENABLE -DCMAKE_INSTALL_PREFIX=$LLVM_INSTALL $LLVM_PATH/llvm" \
							"ninja"
}

_install() {
	__make_dir_and_exec_cmd "$LLVM_BUILD" \
							"ninja install"
}

_target="LLVM-$LLVM_VERSION"