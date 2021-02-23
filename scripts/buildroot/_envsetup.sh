#!/bin/sh -e

__export_envvar "BUILDROOT" "$TOOLCHAINS_DIR/buildroot"
export BUILDROOT_IMAGE_AARCH64="$BUILDROOT_BUILD/images/rootfs.ext3"
export BUILDROOT_VERSION="2020.11.3"
