#!/bin/sh -e

if [ ${PWD#$EXP_DIR} = $PWD ]; then
	while true; do
		echo    "[WARN] You are running the fuzzer outside of EXP_DIR"
		echo    "       EXP_DIR: $EXP_DIR"
		echo    "       PWD    : $PWD"
		read -p "       Do you want to run the fuzzer? [yn] " yn
		case $yn in
			[Yy]* ) break;;
			* ) exit 1;;
		esac
	done
fi

SYZKALLER=$SYZKALLER_INSTALL/syz-manager

if [ -z "$CONFIG" ]; then
	CONFIG="$EXP_DIR/x86_64/syzkaller.cfg"
fi

if [ -n "$DEBUG" ]; then
	_DEBUG="-debug"
	_TEE=${TEE:="$TMP_DIR/log"}
fi

OPTS="-config $CONFIG $_DEBUG"

echo "Run syzkaller"
echo "    config : $CONFIG"
echo "    debug  : $DEBUG"
echo "    options: $OPTS"
echo "    tee    : $_TEE"

sleep 2

if [ -n "$_TEE" ]; then
	exec $SYZKALLER $OPTS 2>&1 | tee $_TEE
else
	exec $SYZKALLER $OPTS
fi
