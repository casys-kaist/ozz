#!/bin/sh -e

UID=$(id -u)
PORT=$(echo "5555 + $UID" | bc -l)

if [ -z "$ARCH" ]; then
	ARCH=x86_64
fi

if [ "$ARCH" = "x86_64" ]; then
	KEY_OPTS="-i $KERNELS_DIR/guest/images/x86_64/stretch.id_rsa"
fi

for _FN in "$@"
do
	echo "Uploading $_FN into /root/$FN"
	FN=$(basename $_FN);
	scp -P $PORT $KEY_OPTS $_FN root@localhost:/root/$FN
done
