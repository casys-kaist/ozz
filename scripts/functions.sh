#!/bin/sh -e

## Usage: __git_clone REPO PATH [REVISION]
__git_clone() {
	if [ "$#" -lt 2 ]; then
		return 1
	fi
	GIT="${GIT:-git}"
	SRC=$1
	DST=$(realpath $2)
	if [ "$#" -gt 2 ] ; then
		REV=$3
		_OPT="--branch $REV"
	fi
	# git clone fails if we already cloned it. Suppress the error
	"$GIT" clone "$SRC" "$DST" $_OPT || echo "[WARN] Failed to clone $SRC"
}

## Usage: __export_envvar NAME BASE
__export_envvar() {
	if [ "$#" -ne 2 ]; then
		return 1
	fi
	NAME="$1"
	BASE="$(realpath $2)"
	eval export "${NAME}_PATH=${BASE}"
	eval export "${NAME}_BUILD=$BASE/build"
	eval export "${NAME}_INSTALL=$BASE/install"
}

## Usage: __append_path PATH
__append_path() {
	# Append the given path only if it is not already in $PATH
	# Ref: https://unix.stackexchange.com/a/124447/247307
	case ":${PATH:-$1}:" in
		*:"$1":*) ;;
		# Append a given path in front of $PATH so we can override
		# existing binaries
		*) PATH="$1:$PATH" ;;
	esac
	export PATH
}

## Usage: __make_dir_and_exec_cmd DIR cmds...
__make_dir_and_exec_cmd() {
	if [ "$#" -lt 1 ]; then
		return 1
	fi
	DIR=$1
	shift 1
	mkdir -p "$DIR"
	for cmd in "$@"; do
		(cd "$DIR"; eval "$cmd")
	done
}

## Usage: __mark_installed TARGET
__mark_installed() {
	if [ -z "$PROJECT_HOME" -o "$#" -lt 1 ]; then
		return 1
	fi
	DIR="$PROJECT_HOME/.installed"
	touch "$DIR/$1"
}

## Usage: __mark_installed TARGET
__check_installed() {
	if [ -z "$PROJECT_HOME" -o "$#" -lt 1 ]; then
		return 1
	fi
	DIR="$PROJECT_HOME/.installed"
	mkdir -p $DIR
	[ -f "$DIR/$1" ]
}
