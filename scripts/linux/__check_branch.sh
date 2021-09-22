#!/bin/sh -e

branch=$(git rev-parse --abbrev-ref HEAD)

__exit() {
	echo "[-]" $1
	if [ -z "$IGNORE" ]; then
		exit 1
	fi
}

__check_symlink() {
	if [ -z "$1" -o -z "$2" ]; then
		__exit "empty \"$1\" or \"$2\"?"
	fi
	if [ ! -f "$1" -a ! -d "$1" ]; then
		__exit "$1 does not exist"
	fi
	if [ ! -h "$2" -o $(readlink -e "$1")'-' != $(readlink -f "$2")'-' ]; then
		__exit "symbolic link $2 is broken"
	fi
}

check_to_be_instrumented_functions() {
	ORIG="$TMP_DIR/to-be-instrumented-functions.lst-$branch"
	LINK="$TMP_DIR/to-be-instrumented-functions.lst"
	__check_symlink $ORIG $LINK
}


check_builddir() {
	ORIG="$KERNEL_X86_64""-$branch"
	LINK="$KERNEL_X86_64"
	__check_symlink $ORIG $LINK
}

check_builddir
check_to_be_instrumented_functions
