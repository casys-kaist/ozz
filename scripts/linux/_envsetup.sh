#!/bin/sh -e

GUEST_DIR="$KERNELS_DIR/guest/"
export KERNEL_X86_64="$GUEST_DIR/builds/x86_64"
export CLANGD_FLAGS="--compile-commands-dir=$KERNEL_X86_64/"
