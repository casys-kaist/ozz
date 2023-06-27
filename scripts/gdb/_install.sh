#!/bin/sh -e

[ -n "$__RELRAZZER_READY" ] || exit 1

. "$SCRIPTS_DIR/gdb/_envsetup.sh"

_TAR="$TMP_DIR/gdb-$GDB_VERSION.tar.xz"

_download() {
	URL="https://ftp.gnu.org/gnu/gdb/gdb-$GDB_VERSION.tar.xz"
	wget $URL -O $_TAR
}

_build() {
	tar xvf "$_TAR" --directory "$TOOLCHAINS_DIR"
	mv "$TOOLCHAINS_DIR/gdb-$GDB_VERSION" "$TOOLCHAINS_DIR/gdb"
	__make_dir_and_exec_cmd "$GDB_BUILD" \
							"$GDB_PATH/configure --prefix=$GDB_INSTALL" \
							"make -j`nproc`"
}

_install() {
	__make_dir_and_exec_cmd "$GDB_BUILD" \
		 					"make install"
}

_target="gdb-$GDB_VERSION"
