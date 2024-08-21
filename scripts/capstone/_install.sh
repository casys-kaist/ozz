#!/bin/sh -e

[ -n "$__RELRAZZER_READY" ] || exit 1

. $SCRIPTS_DIR/capstone/_envsetup.sh

_download() {
	REPO="https://github.com/capstone-engine/capstone.git"
	__git_clone "$REPO" "$CAPSTONE_PATH" "$CAPSTONE_VERSION"
}

_build() {
	TURN_OFF_ALL_ARCHS_OPT="-DCAPSTONE_ARCHITECTURE_DEFAULT=OFF"
	SELECT_X86_OPT="-DCAPSTONE_X86_SUPPORT=1"
	CAPSTONE_OPTS= "$TURN_OFF_ALL_ARCHS_OPT $SELECT_X86_OPT"
	PREFIX_OPT="-DCMAKE_INSTALL_PREFIX=$CAPSTONE_INSTALL"
	__make_dir_and_exec_cmd "$CAPSTONE_BUILD" \
							"cmake $CAPSTONE_OPTS $PREFIX_OPT ../" \
							"ninja"
}

_install() {
	__make_dir_and_exec_cmd "$CAPSTONE_BUILD" \
							"ninja install"
}

_target="capstone-$CAPSTONE_VERSION"
