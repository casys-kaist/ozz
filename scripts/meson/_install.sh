#!/bin/sh -e

[ -n "$__RELRAZZER_READY" ] || exit 1

. "$SCRIPTS_DIR/meson/_envsetup.sh"

_download() {
	REPO="https://github.com/mesonbuild/meson.git"
	__git_clone "$REPO" "$MESON_PATH" "$MESON_VERSION"
}

_build() {
	: # Meson is implemented in Python. Nothing to do
}

_install() {
	__make_dir_and_exec_cmd "$MESON_INSTALL" \
							"ln -s $MESON_PATH/meson.py ./meson"
}

_target="meson-$MESON_VERSION"
