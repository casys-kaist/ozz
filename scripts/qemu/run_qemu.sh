#!/bin/sh -e

$SCRIPTS_DIR/linux/__create_symlinks.sh "linux"
$SCRIPTS_DIR/linux/__check_suffix.sh "linux"

_UID=$(id -u)

MEMORY=8192
PORT=$(echo "5555 + $_UID" | bc -l)
GDBPORT=$(echo "1234 + $_UID" | bc -l)

HMP="-monitor unix:$TMP_DIR/monitor.sock,server,nowait -serial mon:stdio"
QMP="-qmp unix:$TMP_DIR/qmp.sock,server,nowait"

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
	QEMU=$QEMU_X86
	IMAGE="$KERNELS_DIR/guest/images/x86_64/bookworm.img"
	if [ -z $KERNEL ]; then
		KERNEL="$KERNELS_DIR/guest/builds/x86_64/arch/x86_64/boot/bzImage"
	fi
	NETWORK="-netdev user,id=vnet0,hostfwd=tcp::$PORT-:22 \
		-device virtio-net-pci,netdev=vnet0"
	KERNELCMD='console=ttyS0 root=/dev/sda crashkernel=512M selinux=0 null_blk.submit_queues=2'
	MACHINE=
else
	QEMU=$QEMU_ARM
	IMAGE="$KERNELS_DIR/guest/images/arm64/rootfs.ext3"
	KERNEL="$KERNELS_DIR/guest/builds/arm64/arch/arm64/boot/Image"
	NETWORK="-net user,hostfwd=tcp:127.0.0.1:$PORT-:22 -net nic"
	KERNELCMD='console=ttyAMA0 root=/dev/vda oops=panic panic_on_warn=1 panic=-1 ftrace_dump_on_oops=orig_cpu debug earlyprintk=serial slub_debug=UZ'
	# We are using a x86_64 machine
	KVM=
	MACHINE="-machine virt"
fi

echo "Running QEMU on $ARCH"
sleep 3

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
      -gdb tcp::"$GDBPORT" \
	  $KVM 2>&1 | tee $VM_LOGFILE
