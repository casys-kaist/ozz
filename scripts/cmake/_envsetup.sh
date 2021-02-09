#!/bin/sh -e

__export_envvar "CMAKE" "$TOOLCHAINS_DIR/cmake"
__append_path "$CMAKE_INSTALL/bin"
export CMAKE="$CMAKE_INSTALL/bin/cmake"
export CMAKE_VERSION="v3.19.4"
export CMAKE_GENERATOR="Ninja"
