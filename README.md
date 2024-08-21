# Ozz: Identifying Kernel Out-of-Order Concurrency Bugs with In-Vivo Memory Access Reordering

## System Setup

### Caveat

The Ozz's userspace executor needs to invoke hypercalls from the guest
uesrspace, but the deployed kvm module as is rejects hypercalls from
the guest userspace. Thus, the kvm module should be patched to accept
hypercalls from the guest userspace. To enable hypercalls from the
guest userspace, one needs to modify **the host kvm module**.

### Obtaining Source Code

There are two repositories, one for Ozz and the other for the target
kernel which incorporates OEMU's callback functions. The target kernel
repository is added in the Ozz's repository as a submodule. To get the
source code, run the following commands:

```sh
git clone git@github.com:casys-kaist/ozz.git
cd ozz
git submodule update --init --recursive
```

### Modifying the Host KVM Module

To modify the host kvm module, one can apply patches in
`$PROJECT_HOME/kernels/host/patches` into the kernel source code, and
then build and install the kernel (or the kvm module).

In the case of Ubuntu, one can find the repository of the kernel in
[Ubuntu Wiki](https://wiki.ubuntu.com/Kernel/Dev/KernelGitGuide). Here
are some examples:

| Ubuntu Release | Kernel repository |
| --- | --- |
| focal | git://git.launchpad.net/~ubuntu-kernel/ubuntu/+source/linux/+git/focal |
| jammy | git://git.launchpad.net/~ubuntu-kernel/ubuntu/+source/linux/+git/jammy |
| noble | git://git.launchpad.net/~ubuntu-kernel/ubuntu/+source/linux/+git/noble |

Since modifying the host kernel may affect the entire system, we would
like to ask you to carefully read the webpage to sufficiently
understand the consequences before patching the host kvm module.


### Installing Toolchains

To install toolchains needed by Ozz, we provide scripts to install the
toolchains.

First, `cd` into the project directory. In the directory, run the
following command to setup environment variables that are used in the
artifact evaluation process.

```sh
source ./scripts/envsetup.sh
```

After sourcing the script, the `$PROJECT_HOME` environment variable
points to the root directory of the repository.

Then, run the following command to install toolchains.
```sh
./scripts/install.sh
```
It will take a few hours, and occupy tens of GBs (mostly because of LLVM).

All binaries will be installed in `$PROJECT_HOME`, so to remove Ozz,
just remove the `$PROJECT_HOME` directory.


### Installing Ozz

After installing toolchains, one needs to build the Ozz's compiler
pass, the target Linux kernel, a disk iamge, and Ozz.

The Ozz's compiler pass resides in
`$PROJECT_HOME/tools/SoftStoreBufferPass`. To build it, run the
following commands:

```sh
mkdir -p $PROJECT_HOME/tools/SoftStoreBufferPass/build
cd $PROJECT_HOME/tools/SoftStoreBufferPass/build
cmake ..
ninja

# Check the result
→ ls $PROJECT_HOME/tools/SoftStoreBufferPass/build/pass
...
libSSBPass.so      # The LLVM pass shared object
...
```

The target Linux kernel should be built after the Ozz's compiler pass,
as the compiler pass is required to build the kernel. To build the
target Linux kernel, run the following commands:

```sh
cd $PROJECT_HOME
CONFIG=./kernels/guest/configs/config.x86_64 ./scripts/linux/build.sh

# Check the result. vmlinux and bzImage should be there
→ ls $PROJECT_HOME/kernels/guest/builds/x86_64
arch   built-in.a  crypto   fs       init      ipc     lib       mm               modules.builtin.modinfo  Module.symvers  scripts   sound   System.map  usr   vmlinux    vmlinux.o
block  certs       drivers  include  io_uring  kernel  Makefile  modules.builtin  modules.order            net             security  source  tools       virt  vmlinux.a

→ ls $PROJECT_HOME/kernels/guest/builds/x86_64/arch/x86/boot
a20.o       cmdline.o   cpucheck.o  cpustr.h                header.o  mkcpustr  printf.o   setup.elf  tty.o         video-mode.o  video-vga.o  zoffset.h
bioscall.o  compressed  cpuflags.o  early_serial_console.o  main.o    pmjump.o  regs.o     string.o   version.o     video.o       vmlinux.bin
bzImage     copy.o      cpu.o       edd.o                   memory.o  pm.o      setup.bin  tools      video-bios.o  video-vesa.o  voffset.h
```

Ozz requires a disk image to correctly boot up a virtual machine. To
create the disk image, run the following commands.

```sh
cd $PROJECT_HOME/kernels/guest/images/x86_64
./create-image-x86_64.sh

# Check the result. bookworm.img and bookworm.id_rsa are required.
→ ls $PROJECT_HOME/kernels/guest/images/x86_64
bookworm.id_rsa  bookworm.id_rsa.pub  bookworm.img  create-image-x86_64.sh
```

If the script exits with an error message as below,
```
E: Release signed by unknown key (key id F8D2585B8783D481)
   The specified keyring /usr/share/keyrings/debian-archive-keyring.gpg may be incorrect or out of date.
   You can find the latest Debian release key at https://ftp-master.debian.org/keys.html
```
get a new keyring and inform `debootstrap` to use it as follows:
```sh
cd $PROJECT_HOME/kernels/guest/images/x86_64
wget https://ftp-master.debian.org/keys/release-12.asc -qO- | gpg --import --no-default-keyring --keyring ./debian-release-12.gpg
./create-image-x86_64.sh -k ./debian-release-12.gpg
```

Lastly, run the following commands to build Ozz:
```sh
cd $PROJECT_HOME/tools/syzkaller
make

# Check the result.
→ ls $PROJECT_HOME/tools/syzkaller/bin
linux_amd64  syz-db  syz-manager  syz-sysgen

→ ls $PROJECT_HOME/tools/syzkaller/bin/linux_amd64
syz-executor  syz-fuzzer
```

## Running Ozz

Ozz is implemented based on Syzkaller. Thus, a configuration of
Syzkaller should be generated:
```sh
cd $PROJECT_HOME/exp/x86_64
envsubst < templates/syzkaller.cfg.template > syzkaller.cfg
```

Then, run Ozz as follows:
```sh
cd $PROJECT_HOME/exp/x86_64
../scripts/run_fuzzer.sh

Run syzkaller
    syzkaller : ozz/gotools/src/github.com/google/syzkaller/bin/syz-manager
    kernel    : (default) ozz/kernels/guest/builds/x86_64-HEAD
    config    : ozz/exp/x86_64/syzkaller.cfg
    debug     :
    options   :  -config ozz/exp/x86_64/syzkaller.cfg
    tee       : ozz/exp/x86_64/workdir/log
    timestamp :
    workdir   : ozz/exp/x86_64/workdir
[MANAGER] 2024/08/17 19:09:12 loading corpus...
[MANAGER] 2024/08/17 19:09:13 serving http on http://:56741
```

Note that running Ozz for the first time may take a few minutes to
start up. Please wait until it starts running.
