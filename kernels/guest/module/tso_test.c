#include <linux/syscalls.h>
#include <linux/delay.h>

struct shared_t {
	int a;
	int b;
};

static struct shared_t shared;
static struct shared_t result;

#define _DO_SLEEP

__attribute__((softstorebuffer))
void do_tso_thread1(void) {
	struct shared_t *ptr = (struct shared_t *)&shared;
	int la;
	ptr->b = 1;
	la = ptr->a;
#ifdef _DO_SLEEP
	msleep(1000);
#endif
	result.a = la;
}

__attribute__((softstorebuffer))
void do_tso_thread2(void) {
	struct shared_t *ptr = (struct shared_t *)&shared;
	int lb;
	ptr->a = 1;
	lb = ptr->b;
#ifdef _DO_SLEEP
	msleep(1000);
#endif
	result.b = lb;
}

SYSCALL_DEFINE0(tso_thread1) {
	do_tso_thread1();
	return 0;
}

SYSCALL_DEFINE0(tso_thread2) {
	do_tso_thread2();
	return 0;
}

SYSCALL_DEFINE0(tso_init) {
	shared.a = 0;
	shared.b = 0;
	return 0;
}

SYSCALL_DEFINE0(tso_check) {
	BUG_ON(result.a == 0 && result.b == 0);
	return 0;
}
