#!/bin/sh -e

[ -n "$__RELRAZZER_READY" ] || exit 1

. $SCRIPTS_DIR/buildroot/_envsetup.sh

_download() {
	REPO="git@github.com:buildroot/buildroot.git"
	__git_clone "$REPO" "$BUILDROOT_PATH" "$BUILDROOT_VERSION"
}

_build() {
	CONFIG="$SCRIPTS_DIR/buildroot/config"
	__make_dir_and_exec_cmd "$BUILDROOT_PATH" \
							"mkdir -p $BUILDROOT_BUILD" \
							"cp $CONFIG $BUILDROOT_BUILD/.config" \
							"make O=$BUILDROOT_BUILD -j`nproc`"
}

_install() {
	:
}

_target="buildroot-$BUILDROOT_VERSION"
