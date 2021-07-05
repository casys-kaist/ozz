obj-m += vmihelper.o

ifeq ($(ARCH),x86_64)
BUILDDIR=$(KERNEL_X86_64)
else
#TODO: Aarch64
BUILDDIR=
endif

all:
	make -C $(BUILDDIR) M=$(PWD) modules

clean:
	make -C $(BUILDDIR) M=$(PWD) clean
