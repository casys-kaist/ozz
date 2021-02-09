#!/bin/sh -e

[ -n "$__RELRAZZER_READY" ] || exit 1

. "$SCRIPTS_DIR/gcc/_envsetup.sh"

_download() {
	REPO="git://gcc.gnu.org/git/gcc.git"
	_GCC_VERSION="$(echo $GCC_VERSION | sed "s/_/\//")"
	__git_clone "$REPO" "$GCC_PATH" "$_GCC_VERSION"
	(cd $GCC_PATH; ./contrib/download_prerequisites)
}

_build() {
	__make_dir_and_exec_cmd "$GCC_BUILD" \
							"$GCC_PATH/configure --prefix=$GCC_INSTALL --enable-threads=posix --enable-__cxa_atexit --enable-clocale=gnu --enable-languages=c,c++" \
							"make -j`nproc`"
}

_install() {
	__make_dir_and_exec_cmd "$GCC_BUILD" \
							"mkdir -p $GCC_INSTALL" \
							"make install"
}

_target="gcc-$GCC_VERSION"
