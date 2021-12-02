#!/bin/sh -e

__export_envvar "DWARVES" "$TOOLCHAINS_DIR/dwarves"
__append_path "$DWARVES_INSTALL/bin"
if [ -z "$LD_LIBRARY_PATH" ]; then
	export LD_LIBRARY_PATH="$DWARVES_INSTALL/lib"
else
	export LD_LIBRARY_PATH="$DWARVES_INSTALL/lib":$LD_LIBRARY_PATH
fi
export DWARVES_VERSION="v1.19"

