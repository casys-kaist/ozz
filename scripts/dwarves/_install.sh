#!/bin/sh -e

[ -n "$__RELRAZZER_READY" ] || exit 1

. $SCRIPTS_DIR/dwarves/_envsetup.sh

_download() {
	REPO="git@github.com:acmel/dwarves.git"
	__git_clone "$REPO" "$DWARVES_PATH" "$DWARVES_VERSION"
}

_build() {
	# TODO: Install dwarf/elf libraries/tools
	__make_dir_and_exec_cmd "$DWARVES_BUILD" \
							"cmake -D__LIB=lib -DCMAKE_INSTALL_PREFIX=$DWARVES_INSTALL .." \
							"ninja"
}

_install() {
	__make_dir_and_exec_cmd "$DWARVES_BUILD" \
							"ninja install"
}

_target="dwarves-$DWARVES_VERSION"
