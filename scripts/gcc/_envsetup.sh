#!/bin/sh -e

__export_envvar "GCC" "$TOOLCHAINS_DIR/gcc"
__append_path "$GCC_INSTALL/bin"
export GCC="$GCC_INSTALL/bin/gcc"
export GXX="$GCC_INSTALL/bin/g++"
export GCC_VERSION="releases_gcc-10.2.0"
