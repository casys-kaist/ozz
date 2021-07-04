#!/bin/sh -e

[ -n "$__RELRAZZER_READY" ] || exit 1

. $SCRIPTS_DIR/qemu/_envsetup.sh

_download() {
	# Now qemu resides in tools/qemu
	(cd "$QEMU_PATH"; git submodule update --init --recursive)
}

_build() {
	TARGETS="x86_64-softmmu,aarch64-softmmu,riscv64-softmmu,aarch64-linux-user,riscv64-linux-user,x86_64-linux-user"
	_DEPS="--ninja=$NINJA --meson=$MESON --cc=$GCC --cxx=$GXX"
	_OPTS="--enable-curses --enable-kvm --prefix=$QEMU_INSTALL"
	__make_dir_and_exec_cmd "$QEMU_BUILD" \
							"$QEMU_PATH/configure --target-list=$TARGETS $_DEPS $_OPTS" \
							"ninja"
}

_install() {
	__make_dir_and_exec_cmd "$QEMU_BUILD" \
							"ninja install"
}

_target="qemu-$QEMU_VERSION"
