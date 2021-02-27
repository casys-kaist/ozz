#!/bin/sh -e

if [ "$1" = "aarch64" -o "$1" = "arm64" ]; then
	export ARCH="arm64"
	export CROSS_COMPILE="aarch64-linux-gnu-"
	export CONFIG="$PROJECT_HOME/kernels/guest/configs/config.aarch64"
	MEMMODEL="PSO"
elif [ "$1" = "x86_64" ]; then
	# Well, we are not cross-compiling
	export ARCH="x86_64"
	export CONFIG="$PROJECT_HOME/kernels/guest/configs/config.x86_64"
	MEMMODEL="TSO"
else 
	echo "Unknown arch"
	return 1
fi

export LLVM=1
if [ -n "$INSTRUMENT" ]; then
	PASS="$TOOLS_DIR/SoftStoreBufferPass/build/pass/libSSBPass.so"
	export KCFLAGS="-Xclang -load -Xclang $PASS -mllvm -arch=$ARCH -mllvm -memorymodel=$MEMMODEL"
fi
