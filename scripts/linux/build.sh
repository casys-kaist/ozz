#!/bin/sh -e

if [ -z "$CONFIG" ]; then
	echo "\$CONFIG is empty"
	exit 1
fi

if [ -z "$ARCH" ]; then
	echo "\$ARCH is empty"
	exit 1
fi

OUTDIR="$PROJECT_HOME/kernels/guest/builds/$ARCH"
LINUXDIR="$PROJECT_HOME/kernels/guest/linux"

mkdir -p "$OUTDIR"
cp "$CONFIG" "$OUTDIR/.config"
(cd $LINUXDIR; make O=$OUTDIR oldconfig; make O=$OUTDIR -j`nproc`)
