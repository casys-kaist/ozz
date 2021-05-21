#!/bin/sh -e

PORT=5555

if [ -z "$ARCH" ]; then
	ARCH=x86_64
fi

if [ "$ARCH" = "x86_64" ]; then
	KEY_OPTS="-i $KERNELS_DIR/guest/images/x86_64/stretch.id_rsa"
fi

FN=$(basename $1)

echo "Uploading $1 into /root/$FN"

scp -P $PORT $KEY_OPTS $1 root@localhost:/root/$FN
