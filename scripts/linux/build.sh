#!/bin/sh -e

if [ -z "$ARCH" ]; then
	echo "\$ARCH is empty"
	exit 1
fi

OUTDIR="$PROJECT_HOME/kernels/guest/builds/$ARCH"
LINUXDIR="$PROJECT_HOME/kernels/guest/linux"

mkdir -p "$OUTDIR"
if [ -n "$CONFIG" ]; then
	cp "$CONFIG" "$OUTDIR/.config"
fi
(cd $LINUXDIR; make O=$OUTDIR oldconfig; make O=$OUTDIR -j`nproc` "$@")
