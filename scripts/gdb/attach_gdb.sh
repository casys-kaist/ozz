#!/bin/sh -e

[ -n "$PROJECT_HOME" ] || exit 1

BATCHCMD_DEFAULT=$TMP_DIR/gdb-cmds.batch

if [ -z "$ARCH" ]; then
    ARCH="x86_64"
fi

if [ "$ARCH" = "x86_64" ]; then
    TARGET_ARCH="i386:x86-64:intel"
else
    TARGET_ARCH="aarch64"
fi

_UID=$(id -u)
GDBPORT=$(echo "1234 + $_UID" | bc -l)

cat <<EOF > $BATCHCMD_DEFAULT
set architecture $TARGET_ARCH
target remote :1234
set disassemble-next-line on
EOF

if [ "$#" -lt "1" ]; then
    echo "[WARN] Missing a vmlinux path"
    echo "[WARN] Trying \"kernels/guest/builds/$ARCH/vmlinux\""
    echo
    VMLINUX="$PROJECT_HOME/kernels/guest/builds/$ARCH/vmlinux"
else
    VMLINUX=$1
fi

if [ -n "$BATCHCMD" ]; then
    BATCHCMD_ADDITIONAL="-x $BATCHCMD"
fi


set -x
gdb -x $BATCHCMD_DEFAULT $BATCHCMD_ADDITIONAL $VMLINUX
