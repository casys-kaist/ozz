#!/bin/bash -e

[ -n "$PROJECT_HOME" ] || exit 1

BATCHCMD=$PROJECT_HOME/.gdb-cmds.batch

if [ -z "$ARCH" ]; then
	ARCH="x86_64"
fi

if [ "$ARCH" = "x86_64" ]; then
	cat <<EOF > $BATCHCMD
set architecture i386:x86-64:intel
target remote :1234
set disassemble-next-line on
EOF
else
	# TODO: AArch64
	:
fi

if [ "$#" -lt "1" ]; then
	echo "[WARN] Missing a vmlinux path"
	echo "[WARN] Trying \"kernels/guest/builds/$ARCH/vmlinux\""
	echo
	VMLINUX="$PROJECT_HOME/kernels/guest/builds/$ARCH/vmlinux"
else
	VMLINUX=$1
fi

set -x
gdb -x $BATCHCMD $VMLINUX
