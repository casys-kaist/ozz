#!/bin/sh -e

QEMU=qemu-system-x86_64
IMAGE="$KERNELS_DIR/guest/images/x86_64/stretch.img"
MEMORY=2048
KERNEL="$KERNELS_DIR/guest/builds/x86_64/arch/x86_64/boot/bzImage"
PORT=5555
NETWORK="-netdev user,id=vnet0,hostfwd=tcp::$PORT-:22 \
		-device virtio-net-pci,netdev=vnet0"
HMP="-monitor unix:/tmp/monitor.sock,server,nowait -serial mon:stdio"
QMP="-qmp unix:/tmp/qmp.sock,server,nowait"

if [ -z $NO_SNAPSHOT ]; then
	SNAPSHOT="-snapshot"
fi

if [ -z $NO_KVM ]; then
	KVM="-enable-kvm -cpu host"
fi

if [ -z $NUM_CPUS ]; then
	NUM_CPUS=4
fi

$QEMU -smp cpus=$NUM_CPUS \
	  -append 'console=ttyS0 root=/dev/sda crashkernel=512M selinux=0' \
	  -nographic \
	  -hda $IMAGE \
	  -m $MEMORY \
	  -kernel $KERNEL \
	  $NETWORK \
	  $HMP \
	  $QMP\
	  $SNAPSHOT \
      -s \
	  $KVM 2>&1 | tee $VM_LOGFILE
