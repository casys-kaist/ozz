obj-m += memmodel-tests.o
memmodel-tests-objs := tso_test.o pso_test.o

all:
	make -C /lib/modules/$(shell uname -r)/build M=$(PWD) modules

clean:
	make -C /lib/modules/$(shell uname -r)/build M=$(PWD) clean
