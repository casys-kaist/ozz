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
SNAPSHOT="-snapshot"
KVM="-enable-kvm"

$QEMU -smp cpus=4 \
	  -cpu host \
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
