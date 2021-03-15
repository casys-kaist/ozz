#!/bin/sh -e

__export_envvar "SYZKALLER" "$GOTOOLS_DIR/src/github.com/google/syzkaller"
__append_path "$SYZKALLER_PATH/bin"

GUEST_DIR="$KERNELS_DIR/guest/"

# Used in the syzkaller config
export IMAGE_X86_64="$GUEST_DIR/images/x86_64"
export KERNEL_X86_64="$GUEST_DIR/builds/x86_64"
export NR_VMS=`expr $(nproc) / 2`
