#!/bin/sh -e

QEMU=qemu-system-aarch64
IMAGE="$KERNELS_DIR/guest/images/arm64/rootfs.ext3"
MEMORY=2048
KERNEL="$KERNELS_DIR/guest/builds/arm64/arch/arm64/boot/Image"
PORT=5555
NETWORK="-net user,hostfwd=tcp:127.0.0.1:$PORT-:22 -net nic"
HMP="-monitor unix:/tmp/monitor.sock,server,nowait -serial mon:stdio"
QMP="-qmp unix:/tmp/qmp.sock,server,nowait"
SNAPSHOT="-snapshot"

$QEMU -smp 2 \
      -machine virt \
      -cpu cortex-a57 \
      -nographic \
      -hda $IMAGE \
      -m $MEMORY \
      -kernel $KERNEL \
      -append "console=ttyAMA0 root=/dev/vda oops=panic panic_on_warn=1 panic=-1 ftrace_dump_on_oops=orig_cpu debug earlyprintk=serial slub_debug=UZ" \
      $NETWORK \
      $HMP \
      $QMP\
      $SNAPSHOT \
      -s 2>&1 | tee $VM_LOGFILE
