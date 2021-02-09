#!/bin/sh -e

__export_envvar "NINJA" "$TOOLCHAINS_DIR/ninja"
__append_path "$NINJA_INSTALL/bin"
export NINJA="$NINJA_INSTALL/bin/ninja"
export NINJA_VERSION="v1.10.2"
