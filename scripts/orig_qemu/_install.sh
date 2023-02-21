#!/bin/sh -e

[ -n "$__RELRAZZER_READY" ] || exit 1

. "$SCRIPTS_DIR/orig_qemu/_envsetup.sh"

_TAR="$TMP_DIR/qemu-$ORIG_QEMU_VERSION.tar.xz"

_download() {
	URL="https://download.qemu.org/qemu-$ORIG_QEMU_VERSION.tar.xz"
	wget $URL -O $_TAR
	tar xvf "$_TAR" --directory "$TOOLCHAINS_DIR"
	mv "$TOOLCHAINS_DIR/qemu-$ORIG_QEMU_VERSION" "$TOOLCHAINS_DIR/orig_qemu"
}

_build() {
	PRE_COMMAND=
	case $1 in
		*"--rebuild"*)
			PRE_COMMAND="ninja clean" ;;
	esac
	# XXX: copy from scripts/qemu/_install.sh
	TARGETS="x86_64-softmmu"
	_DEPS="--ninja=$NINJA --meson=$MESON --cc=$GCC --cxx=$GXX --gdb=$GDB"
	_OPTS="--enable-curses --enable-kvm --enable-debug --prefix=$ORIG_QEMU_INSTALL $OPTS"
	__make_dir_and_exec_cmd "$ORIG_QEMU_BUILD" \
							"$ORIG_QEMU_PATH/configure --target-list=$TARGETS $_DEPS $_OPTS" \
							"$PRE_COMMAND" \
							"ninja"
}

_install() {
	__make_dir_and_exec_cmd "$ORIG_QEMU_BUILD" \
		 					"make install"
}

_target="orig-qemu-$ORIG_QEMU_VERSION"
