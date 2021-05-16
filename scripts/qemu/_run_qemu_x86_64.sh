#!/bin/sh -e

echo "Use run_qemu.sh instead"
exit 1

MEMORY=2048
PORT=5555
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

if [ -z $ARCH ]; then
	ARCH=x86_64
fi

if [ $ARCH = "x86_64" ]; then
	QEMU=qemu-system-x86_64
	IMAGE="$KERNELS_DIR/guest/images/x86_64/stretch.img"
	KERNEL="$KERNELS_DIR/guest/builds/x86_64/arch/x86_64/boot/bzImage"
	NETWORK="-netdev user,id=vnet0,hostfwd=tcp::$PORT-:22 \
		-device virtio-net-pci,netdev=vnet0"
	KERNELCMD='console=ttyS0 root=/dev/sda crashkernel=512M selinux=0'
	MACHINE=
else
	QEMU=qemu-system-aarch64
	IMAGE="$KERNELS_DIR/guest/images/arm64/rootfs.ext3"
	KERNEL="$KERNELS_DIR/guest/builds/arm64/arch/arm64/boot/Image"
	NETWORK="-net user,hostfwd=tcp:127.0.0.1:$PORT-:22 -net nic"
	KERNELCMD='console=ttyAMA0 root=/dev/vda oops=panic panic_on_warn=1 panic=-1 ftrace_dump_on_oops=orig_cpu debug earlyprintk=serial slub_debug=UZ'
	# We are using a x86_64 machine
	KVM=
	MACHINE="-machine virt"
fi


$QEMU -smp cpus=$NUM_CPUS \
	  -append "$KERNELCMD" \
	  -nographic \
	  -hda $IMAGE \
	  -m $MEMORY \
	  -kernel $KERNEL \
	  $NETWORK \
	  $HMP \
	  $QMP\
	  $SNAPSHOT \
	  $MACHINE \
      -s \
	  $KVM 2>&1 | tee $VM_LOGFILE
