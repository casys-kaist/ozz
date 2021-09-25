#!/bin/sh -e

mkdir -p "$TOOLCHAINS_DIR/python"

__export_envvar "PYTHON" "$TOOLCHAINS_DIR/python/python"
__append_path "$PYTHON_INSTALL/bin"
export PYTHON="$PYTHON_INSTALL/bin/python"
export PYTHON_VERSION="3.9.7"
