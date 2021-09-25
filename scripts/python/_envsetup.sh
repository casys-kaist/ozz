#!/bin/sh -e

mkdir -p "$TOOLCHAINS_DIR/python"

__export_envvar "PYTHON" "$TOOLCHAINS_DIR/python/python"
__append_path "$PYTHON_INSTALL/bin"
export PYTHON="$PYTHON_INSTALL/bin/python"
export PYTHON_VERSION="3.9.7"
export PYTHON_VIRTENV_PATH="$(dirname $PYTHON_PATH)/virtenv"
export PYTHON_VIRTENV_ACTIVATE="$(dirname $PYTHON_PATH)/virtenv/bin/activate"

if [ -f "$PYTHON_VIRTENV_ACTIVATE" ]; then
	. $PYTHON_VIRTENV_ACTIVATE
fi
