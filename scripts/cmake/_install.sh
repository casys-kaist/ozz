#!/bin/sh -e

[ -n "$__RELRAZZER_READY" ] || exit 1

. $SCRIPTS_DIR/cmake/_envsetup.sh

_download() {
	REPO="git@github.com:Kitware/CMake.git"
	__git_clone $REPO $CMAKE_PATH $CMAKE_VERSION
}

_build() {
	__make_dir_and_exec_cmd "$CMAKE_BUILD" \
							"$CMAKE_PATH/bootstrap --generator=\"Unix Makefiles\" --prefix=$CMAKE_INSTALL" \
							"make -j`nproc`"
}

_install() {
	__make_dir_and_exec_cmd "$CMAKE_BUILD" \
							"mkdir -p $CMAKE_INSTALL" \
							"make install"
}

_target="cmake-$CMAKE_VERSION"
