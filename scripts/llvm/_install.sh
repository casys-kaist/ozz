#!/bin/sh -e

[ -n "$__RELRAZZER_READY" ] || exit 1

. "$SCRIPTS_DIR/llvm/_envsetup.sh"

_download() {
	REPO="https://github.com/llvm/llvm-project.git"
	__git_clone $REPO $LLVM_PATH $LLVM_VERSION
}

_build() {
	# Apply patches first
	__git_am "$LLVM_PATH" "$SCRIPTS_DIR/llvm/patches"
	# Then build LLVM
	_ENABLE="-DLLVM_ENABLE_PROJECTS=clang\;compiler-rt"
	_OPTIONS="-DCMAKE_INSTALL_PREFIX=$LLVM_INSTALL -DCMAKE_BUILD_TYPE=RelWithDebInfo -DLLVM_ENABLE_ASSERTIONS=ON"
	__make_dir_and_exec_cmd "$LLVM_BUILD" \
							"cmake -G 'Ninja' $_ENABLE $_OPTIONS $LLVM_PATH/llvm" \
							"ninja"
}

_install() {
	__make_dir_and_exec_cmd "$LLVM_BUILD" \
							"ninja install"
}

_target="LLVM-$LLVM_VERSION"
