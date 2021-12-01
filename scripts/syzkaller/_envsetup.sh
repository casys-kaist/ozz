#!/bin/sh -e

__export_envvar "SYZKALLER" "$GOTOOLS_DIR/src/github.com/google/syzkaller"
__export_envvar "SYZKALLER_BASELINE" "$GOTOOLS_DIR/src/github.com/google/syzkaller-baseline"
__append_path "$SYZKALLER_PATH/bin"

export SYZKALLER_BUILD=$SYZKALLER_PATH/bin
export SYZKALLER_INSTALL=$SYZKALLER_BUILD

export SYZKALLER_BASELINE_BUILD=$SYZKALLER_BASELINE_PATH/bin
export SYZKALLER_BASELINE_INSTALL=$SYZKALLER_BASELINE_BUILD

GUEST_DIR="$KERNELS_DIR/guest/"

# Used in the syzkaller config
export IMAGE_X86_64="$GUEST_DIR/images/x86_64"
export KERNEL_X86_64="$GUEST_DIR/builds/x86_64"
export NR_VMS=`expr $(nproc) / 2`

# Used for the baseline
export KERNEL_X86_64_BASELINE="$GUEST_DIR/builds/x86_64-baseline"
