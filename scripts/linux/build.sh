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
	_CONFIG="$CONFIG"
	COPY_CONFIG=1
else
	_CONFIG="$KERNELS_DIR/guest/configs/config.$ARCH"
fi

if [ -n "$COPY_CONFIG" -o ! -f "$OUTDIR/.config" ]; then
	echo "copy $_CONFIG to $OUTDIR/.config"
	cp "$_CONFIG" "$OUTDIR/.config"
fi

if [ -z "$NPROC" ]; then
	NPROC=$(expr `nproc` / 2)
fi

(cd $LINUXDIR; make O=$OUTDIR oldconfig; make O=$OUTDIR -j"$NPROC" "$@")

if [ -n "$_DEDUP" ]; then
	# TODO: do this inline
	FN=$(readlink -f "$TMP_DIR/to-be-instrumented-functions.lst")
	TN="$TMP_DIR/to-be-instrumented-functions.lst__temporary"
	sort "$FN" | uniq -u > "$TN"
	mv "$TN" "$FN"
fi
