#!/bin/sh -e

mkdir_env() {
	mkdir -p $1
	echo $1
}

## Variables for the project's directory layout
## HOME
## |-exp
## |-scripts
## |-kernels
## |-gotools(=$GOPATH)
## |-tools
## |-toolchains
## |-(tmp)
if [ -n "$BASH_SOURCE" ]; then
	export SCRIPTS_DIR=$(mkdir_env "$(cd "$(dirname "$BASH_SOURCE")"; pwd)")
else
	export SCRIPTS_DIR=$(mkdir_env "$(cd "$(dirname "$0")"; pwd)")
fi
export PROJECT_HOME=$(mkdir_env "$(dirname $SCRIPTS_DIR)")
export EXP_DIR=$(mkdir_env "$PROJECT_HOME/exp")
export KERNELS_DIR=$(mkdir_env "$PROJECT_HOME/kernels")
export GOTOOLS_DIR=$(mkdir_env "$PROJECT_HOME/gotools")
export TOOLS_DIR=$(mkdir_env "$PROJECT_HOME/tools")
export TOOLCHAINS_DIR=$(mkdir_env "$PROJECT_HOME/toolchains")
export TMP_DIR=$(mkdir_env "$PROJECT_HOME/tmp")

## Import handful functions
. "$SCRIPTS_DIR/functions.sh"

## Variables for each subproject
for _PROJ in `find $SCRIPTS_DIR -mindepth 1 -maxdepth 1 -type d`;
do
	PROJ=$(realpath $_PROJ)
	if [ -f "$PROJ/_envsetup.sh" ]; then
		. "$PROJ/_envsetup.sh"
	fi
done

## Now we are ready
export __RELRAZZER_READY=1
