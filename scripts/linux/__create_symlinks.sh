#!/bin/sh -e

if [ -z $SUFFIX ]; then
	_SUFFIX=$(cd $KERNELS_DIR/linux; git rev-parse --abbrev-ref HEAD)
else
	_SUFFIX=$SUFFIX
fi

__exit() {
	echo "[-]" $1
	if [ -z "$IGNORE" ]; then
		exit 1
	fi
}

__append_suffix() {
	echo "$1-$_SUFFIX"
}

__create_symlink() {
	SRC="$1"
	DST="$2"
	if [ ! -e "$SRC" ]; then
		__exit "$SRC doest not exist"
	fi
	if [ -e "$DST" -a ! -h "$DST" ]; then
		__exit "cannot create symbolic link $DST"
	fi
	ln -sf -T "$SRC" "$DST"
}

create_builddir_symlink() {
	SRC="$(__append_suffix $KERNEL_X86_64)"

	mkdir -p "$SRC"

	__create_symlink "$SRC" "$KERNEL_X86_64"
}

create_to_be_instrumented_functions_symlink() {
	FILENAME="$TMP_DIR/to-be-instrumented-functions.lst"
	SRC="$KERNELS_DIR/guest/instrument.lst"

	if [ -f "$TMP_DIR/kssb_rebuild" -a -n "$_FIRSTPASS" ]; then
		mv "$SRC" "$SRC".old || true
	fi

	touch $SRC

	__create_symlink "$SRC" "$FILENAME"
}

if [ "$1" = "all" ]; then
	create_builddir_symlink
	create_to_be_instrumented_functions_symlink
elif [ "$1" = "linux" ]; then
	create_builddir_symlink
fi
