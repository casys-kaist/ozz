#!/bin/sh -e

branch=$(git rev-parse --abbrev-ref HEAD)

__exit() {
	echo "[-]" $1
	if [ -z "$IGNORE" ]; then
		exit 1
	fi
}

__append_branch() {
	echo "$1-$branch"
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
	SRC="$(__append_branch $KERNEL_X86_64)"

	mkdir -p "$SRC"

	__create_symlink "$SRC" "$KERNEL_X86_64"
}

create_to_be_instrumented_functions_symlink() {
	FILENAME="$TMP_DIR/to-be-instrumented-functions.lst"
	SRC="$(__append_branch $FILENAME)"

	touch $SRC

	__create_symlink "$SRC" "$FILENAME"
}

create_builddir_symlink
create_to_be_instrumented_functions_symlink
