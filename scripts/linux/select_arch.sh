#!/bin/sh -e

if [ "$1" = "aarch64" -o "$1" = "arm64" ]; then
	export ARCH="arm64"
	export CROSS_COMPILE="aarch64-linux-gnu-"
	export CONFIG="$PROJECT_HOME/kernels/guest/configs/config.aarch64"
elif [ "$1" = "x86_64" ]; then
	# Well, we are not cross-compiling
	export CONFIG="$PROJECT_HOME/kernels/guest/configs/config.x86_64"
else 
	echo "Unknown arch"
	return 1
fi

