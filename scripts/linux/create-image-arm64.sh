#!/bin/sh -e

if [ ! -f "$BUILDROOT_IMAGE_AARCH64" ]; then
	echo "Build buildroot first"
	return 1
fi

ln -s "$BUILDROOT_IMAGE_AARCH64" .
