#!/bin/sh -e

# XXX: This seems a bit weird. No matter the architecture is, we
# emulate the PSO memory model. All we want is to cross compile the
# kernel for aarch64, and x86_64 is used to only for debugging and
# testing.
MEMMODEL="PSO"

if [ "$1" = "aarch64" -o "$1" = "arm64" ]; then
	export ARCH="arm64"
	export CROSS_COMPILE="aarch64-linux-gnu-"
elif [ "$1" = "x86_64" ]; then
	export ARCH="x86_64"
	# Well, we are not cross-compiling
	export CROSS_COMPILE=
else 
	echo "Unknown arch"
	return 1
fi

export LLVM=1
if [ -n "$INSTRUMENT" ]; then
	PASS="$TOOLS_DIR/SoftStoreBufferPass/build/pass/libSSBPass.so"
	export CFLAGS_KSSB="-Xclang -load -Xclang $PASS -mllvm -arch=$ARCH -mllvm -memorymodel=$MEMMODEL"
	export CFLAGS_KSSB_FLUSHONLY="-Xclang -load -Xclang $PASS -mllvm -arch=$ARCH -mllvm -memorymodel=$MEMMODEL -mllvm -ssb-flush-only=true"
fi
