#!/bin/sh -e

MEMMODEL="PSO"

export ARCH="x86_64"
export LLVM=1

PASS="$TOOLS_DIR/SoftStoreBufferPass/build/pass/libSSBPass.so"
export CFLAGS_KSSB="-Xclang -load -Xclang $PASS -mllvm -arch=$ARCH -mllvm -memorymodel=$MEMMODEL"
export CFLAGS_KSSB_FLUSHONLY="-Xclang -load -Xclang $PASS -mllvm -arch=$ARCH -mllvm -memorymodel=$MEMMODEL -mllvm -ssb-flush-only=true"
if [ -n "$FIRSTPASS" ]; then
	export CFLAGS_KSSB="$CFLAGS_KSSB -mllvm -ssb-second-pass=false"
	export _FIRSTPASS=1
	export _DEDUP=1
else
	unset _FIRSTPASS
	unset _DEDUP
fi
