#!/bin/sh -e

__export_envvar "DWARVES" "$TOOLCHAINS_DIR/dwarves"
__append_path "$DWARVES_INSTALL/bin"
export LD_LIBRARY_PATH="$DWARVES_INSTALL/lib":$LD_LIBRARY_PATH
export DWARVES_VERSION="v1.19"

