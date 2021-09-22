#!/bin/sh -e

if [ -z "$ARCH" ]; then
	echo "\$ARCH is empty"
	exit 1
fi

SCRIPTS_LINUX_DIR="$SCRIPTS_DIR/linux/"
$SCRIPTS_LINUX_DIR/__create_symlinks.sh
$SCRIPTS_LINUX_DIR/__check_branch.sh

OUTDIR="$PROJECT_HOME/kernels/guest/builds/$ARCH"
LINUXDIR="$PROJECT_HOME/kernels/guest/linux"

mkdir -p "$OUTDIR"
if [ -n "$CONFIG" ]; then
	cp "$CONFIG" "$OUTDIR/.config"
fi

if [ -z "$NPROC" ]; then
	NPROC=$(expr `nproc` / 2)
fi

(cd $LINUXDIR; make O=$OUTDIR oldconfig; make O=$OUTDIR -j"$NPROC" "$@")
