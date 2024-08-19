#!/bin/sh -e

_UID=$(id -u)
PORT=$(echo "5555 + $_UID" | bc -l)

if [ -z "$ARCH" ]; then
	ARCH=x86_64
fi

if [ "$ARCH" = "x86_64" ]; then
	KEY_OPTS="-i $KERNELS_DIR/guest/images/x86_64/bookworm.id_rsa"
fi

for _FN in "$@"
do
	echo "Uploading $_FN into /root/$FN"
	FN=$(basename $_FN);
	scp -r -P $PORT $KEY_OPTS $_FN root@127.0.0.1:/root/
done
