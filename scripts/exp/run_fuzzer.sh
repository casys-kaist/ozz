#!/bin/sh -e

if [ "${PWD#$EXP_DIR}"/ = "$PWD/" ]; then
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

baseline=0
if [ -n "$BASELINE" ]; then
	baseline=1
fi

SCRIPTS_LINUX_DIR="$SCRIPTS_DIR/linux/"
$SCRIPTS_LINUX_DIR/__create_symlinks.sh "linux"
$SCRIPTS_LINUX_DIR/__check_suffix.sh "linux"

if [ "$baseline" -eq 1 ]; then
	SYZKALLER=$SYZKALLER_BASELINE_INSTALL/syz-manager
	_KERNEL=$KERNEL_X86_64_BASELINE
else
	SYZKALLER=$SYZKALLER_INSTALL/syz-manager
	_KERNEL=$KERNEL_X86_64
fi

if [ -z "$CONFIG" ]; then
	if [ "$baseline" -eq 1 ]; then
		CONFIG="$EXP_DIR/x86_64/baseline.cfg"
	else
		CONFIG="$EXP_DIR/x86_64/syzkaller.cfg"
	fi
fi

WORKDIR=$(cat $CONFIG | grep --extended-regexp "\"workdir\": \"(.*)\"" --only-matching | cut --delimiter=":" --fields=2 | sed s/\"//g)
WORKDIR=${WORKDIR## }

if [ -n "$DEBUG" ]; then
	_DEBUG="-debug"
	TS=1
	BENCH=1
fi

if [ -n "$BENCH" ]; then
	_BENCH="-bench $WORKDIR/bench-$(date +%y%m%d-%H%M%S).txt"
fi

mkdir -p "$WORKDIR"
_TEE=${TEE:="$WORKDIR/log"}
mv "$_TEE" "$_TEE".old || true

OPTS="$OPTS -config $CONFIG $_DEBUG $_BENCH"

echo "Run syzkaller"
echo "    syzkaller : $SYZKALLER"
echo "    kernel    : (default) $(readlink -f $_KERNEL)"
echo "    config    : $CONFIG"
echo "    debug     : $DEBUG"
echo "    options   : $OPTS"
echo "    tee       : $_TEE"
echo "    timestamp : $TS"
echo "    workdir   : $WORKDIR"

sleep 2

if [ -n "$TS" ]; then
	exec $SYZKALLER $OPTS 2>&1 | ts "[%Y-%m-%d %H:%M:%.S]" | tee $_TEE
else
	exec $SYZKALLER $OPTS 2>&1 | tee $_TEE
fi
