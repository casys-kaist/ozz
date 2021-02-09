#!/bin/sh -e

__export_envvar "MESON" "$TOOLCHAINS_DIR/meson"
__append_path "$MESON_INSTALL"
export MESON="$MESON_INSTALL/meson"
export MESON_VERSION=0.56.0

