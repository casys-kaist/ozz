#include <linux/syscalls.h>
#include <linux/slab.h>
#include <linux/delay.h>

MODULE_LICENSE("GPL");
MODULE_AUTHOR("Dae R. Jeong");
MODULE_DESCRIPTION("A simple example to test softstorebuffer for TSO");
MODULE_VERSION("0.01");

struct shared_t {
	int *ptr;
	bool ready;
};

static struct shared_t shared;

#define _DO_SLEEP

SYSCALL_DEFINE0(pso_writer) {
	struct shared_t *ptr = (struct shared_t *)&shared;
	ptr->ptr = (int *)kmalloc(sizeof(*ptr->ptr), GFP_KERNEL);
	ptr->ready = true;
#ifdef _DO_SLEEP
	msleep(3000);
#endif
	return 0;
}

SYSCALL_DEFINE0(pso_reader) {
	struct shared_t *ptr = (struct shared_t *)&shared;
#ifdef _DO_SLEEP
	msleep(1000);
#endif
	if (ptr->ready)
		(void)*ptr->ptr;
	return 0;
}

SYSCALL_DEFINE0(pso_clear) {
	kfree(shared.ptr);
	return 0;
}
