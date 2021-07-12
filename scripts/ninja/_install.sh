#!/bin/sh -e

[ -n "$__RELRAZZER_READY" ] || exit 1

. "$SCRIPTS_DIR/ninja/_envsetup.sh"

_download() {
	REPO="git@github.com:ninja-build/ninja.git"
	__git_clone "$REPO" "$NINJA_PATH" "$NINJA_VERSION"
}

_build() {
	CMAKE=${CMAKE:-cmake}
	__make_dir_and_exec_cmd "$NINJA_BUILD" \
							"$CMAKE -DCMAKE_INSTALL_PREFIX=$NINJA_INSTALL -G 'Unix Makefiles' ../" \
							"make -j`nproc`"
}

_install() {
	__make_dir_and_exec_cmd "$NINJA_BUILD" \
							"mkdir -p $NINJA_INSTALL" \
							"make install"
}

_target="ninja-$NINJA_VERSION"
