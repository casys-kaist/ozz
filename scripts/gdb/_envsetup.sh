#!/bin/sh -e

__export_envvar "GDB" "$TOOLCHAINS_DIR/gdb"
__append_path "$GDB_INSTALL/bin"
export GDB="$GDB_INSTALL/bin/gdb"
export GDB_VERSION=12.1
