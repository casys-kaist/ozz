#!/bin/sh -e

[ -n "$__RELRAZZER_READY" ] || exit 1

. "$SCRIPTS_DIR/python/_envsetup.sh"

_F="python.tar.xz"
_DST="$TMP_DIR/$_F"

_download() {
	URL="https://www.python.org/ftp/python/$PYTHON_VERSION/Python-$PYTHON_VERSION.tar.xz"
	wget "$URL" -O "$_DST"
}

_build() {
	tar xf "$_DST" -C "$TOOLCHAINS_DIR/python"
	mv "$TOOLCHAINS_DIR/python/Python-$PYTHON_VERSION" "$PYTHON_PATH"
	__make_dir_and_exec_cmd "$PYTHON_BUILD" \
							"../configure --enable-optimizations --prefix=$PYTHON_INSTALL --enable-loadable-sqlite-extensions" \
							"make -j`nproc`"
}

_install() {
	_PIP="$PYTHON_VIRTENV_PATH/bin/pip"
	__make_dir_and_exec_cmd "$PYTHON_BUILD" \
							"mkdir -p $PYTHON_INSTALL" \
							"make install" \
							"ln -s $PYTHON_INSTALL/bin/python3 $PYTHON_INSTALL/bin/python" \
							"python -m venv $PYTHON_VIRTENV_PATH" \
							". $PYTHON_VIRTENV_ACTIVATE" \
							"$_PIP install --upgrade pip" \
							"$_PIP install -r $SCRIPTS_DIR/python/requirements.txt"
}

_target="python-$PYTHON_VERSION"
