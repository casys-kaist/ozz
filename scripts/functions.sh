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
	if [ -z "$FULL_HISTORY_CLONE" ] ; then
		_OPT="$_OPT --depth 1"
	fi
	# git clone fails if we already cloned it. Suppress the error
	"$GIT" clone "$SRC" "$DST" $_OPT || echo "[WARN] Failed to clone $SRC"
}

## Usage: __git_am LOCAL_REPO PATCH_DIR
__git_am() {
	if [ "$#" -lt 2 ]; then
		return 1
	fi
	GIT="${GIT:-git}"
	_LOCAL=$(realpath $1)
	_PATCH_DIR=$(realpath $2)
	for _PATCH in `find $_PATCH_DIR -name "*.patch" | sort`;
	do
		(cd $_LOCAL; git am $_PATCH)
	done
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

## Usage: __check_installed TARGET [FORCE]
__check_installed() {
	if [ -z "$PROJECT_HOME" -o "$#" -lt 1 ]; then
		return 1
	fi
	DIR="$PROJECT_HOME/.installed"
	mkdir -p $DIR
	[ -z "$2" -a -f "$DIR/$1" ]
}

## Usage: __contains STRING SUBSTRING
## Ref: https://stackoverflow.com/questions/2829613/how-do-you-tell-if-a-string-contains-another-string-in-posix-sh
__contains() {
    string="$1"
    substring="$2"
    if [ "${string#*"$substring"}" != "$string" ]; then
        echo 1    # $substring is in $string
    else
        echo 0    # $substring is not in $string
    fi
}

__check_config() {
	config_file="$1"
	configs_to_check="$2"
	for _o in $(echo $configs_to_check)
	do
		o="CONFIG_$_o=y"
		if ! grep -q "$o" "$config_file"; then
			printf '%s\n' "$o"
		fi
	done
}

count_item() {
   echo $(IFS=' '; set -f -- $1; echo $#)
}
