#define _GNU_SOURCE

#include <stdio.h>
#include <unistd.h>
#include <stdlib.h>
#include <errno.h>
#include <sched.h>
#include <stdbool.h>
#include <pthread.h>
#include <syscall.h>
#include <sys/types.h>

#define sys_setup_schedule 505
#define sys_clear_schedule 506
#define sys_freeze_schedule 507
#define sys_unfreeze_schedule 508
#define sys_kcsched_test 509

struct kcschedpoint {
	unsigned long addr;
	int order;
} scheds[3] = {
	{0xabcd, 0},
	{0x1234, 1},
	{0xdeadbeaf, 2}
};

struct test_arg {
	struct kcschedpoint *bp;
	int num_bp;
	int cpu;
	bool freeze;
	bool activate;
	bool execute_syscall;
	bool execute_syscall_twice;
};

#define gettid() syscall(SYS_gettid)

void *thr(void *_arg)
{
	struct test_arg *arg = (struct test_arg *)_arg;
	cpu_set_t set;
	CPU_ZERO(&set);
	CPU_SET(arg->cpu, &set);
	if (sched_setaffinity(gettid(), sizeof(set), &set))
		perror("sched_setaffinity");

	if (syscall(sys_setup_schedule, 1, arg->num_bp, arg->bp))
		perror("setup_schedule");

	sleep(1);

	if (arg->freeze)
		if(syscall(sys_freeze_schedule, 1, arg->activate))
			perror("freeze_schedule");

	if (arg->execute_syscall)
		if (syscall(sys_kcsched_test))
			perror("kcsched_test");

	if (arg->execute_syscall_twice)
		if (syscall(sys_kcsched_test))
			perror("kcsched_test");

	if (arg->freeze)
		if(syscall(sys_unfreeze_schedule, 1, arg->activate))
			perror("unfreeze_schedule");

	if (syscall(sys_clear_schedule, 1))
		perror("clear_schedule");

	return NULL;
}

void test_setup_and_clear_schedule(void)
{
	printf("%s\n", __func__);
	struct test_arg arg = {
		.bp = scheds,
		.num_bp = 3,
		.cpu = 0,
		.freeze = false,
		.activate = false,
		.execute_syscall = false,
		.execute_syscall_twice = false,
	};
	thr(&arg);
}

void test_setup_and_clear_twice_schedule(void) {
	printf("%s\n", __func__);
	test_setup_and_clear_schedule();
	test_setup_and_clear_schedule();
}

void test_setup_and_freeze_and_unfreeze_and_clear_schedule(void) {
	printf("%s\n", __func__);
	struct test_arg arg = {
		.bp = scheds,
		.num_bp = 3,
		.cpu = 0,
		.freeze = true,
		.activate = true,
		.execute_syscall = false,
		.execute_syscall_twice = false,
	};
	thr(&arg);
}

unsigned long sys_kcsched_test_addr(void) {
	char buf[128];
	FILE *fp = popen("grep '__do_sys_kcsched_test' /proc/kallsyms | head -n 1 | cut -d' ' -f1", "r");
	fgets(buf, sizeof(buf), fp);
	pclose(fp);
	return strtoul(buf, NULL, 16);
}

void test_breakpoint(void) {
	printf("%s\n", __func__);
	unsigned long addr = sys_kcsched_test_addr();
	struct kcschedpoint bp = {
		.addr = addr,
		.order = 0,
	};

	struct test_arg arg = {
		.bp = &bp,
		.num_bp = 1,
		.cpu = 0,
		.freeze = true,
		.activate = true,
		.execute_syscall = true,
		.execute_syscall_twice = false,
	};
	thr(&arg);
}

void test_breakpoint_twice(void) {
	printf("%s\n", __func__);
	unsigned long addr = sys_kcsched_test_addr();
	struct kcschedpoint bp = {
		.addr = addr,
		.order = 0,
	};

	struct test_arg arg = {
		.bp = &bp,
		.num_bp = 1,
		.cpu = 0,
		.freeze = true,
		.activate = true,
		.execute_syscall = true,
		.execute_syscall_twice = true,
	};
	thr(&arg);
}

void test_breakpoint_two_threads(void) {
	printf("%s\n", __func__);
	unsigned long addr = sys_kcsched_test_addr();
	struct kcschedpoint bp0 = {
		.addr = addr,
		.order = 0,
	};
	struct test_arg arg0 = {
		.bp = &bp0,
		.num_bp = 1,
		.cpu = 0,
		.freeze = true,
		.activate = true,
		.execute_syscall = true,
		.execute_syscall_twice = false,
	};
	struct kcschedpoint bp1 = {
		.addr = addr,
		.order = 1,
	};
	struct test_arg arg1 = {
		.bp = &bp1,
		.num_bp = 1,
		.cpu = 1,
		.freeze = true,
		.activate = true,
		.execute_syscall = true,
		.execute_syscall_twice = false,
	};

	pthread_t pth1, pth2;
	if (pthread_create(&pth1, NULL, thr, (void *)&arg0))
		perror("pthread_create1"), exit(1);
	if (pthread_create(&pth2, NULL, thr, (void *)&arg1))
		perror("pthread_create2"), exit(1);
	if (pthread_join(pth1, NULL))
		perror("pthread_join1"), exit(1);
	if (pthread_join(pth2, NULL))
		perror("pthread_join2"), exit(1);
}

int main(int arch, char *argv[])
{
	/* Benign behaviors */
	test_setup_and_clear_schedule();
	test_setup_and_clear_twice_schedule();
	test_setup_and_freeze_and_unfreeze_and_clear_schedule();
	test_breakpoint();
	test_breakpoint_twice();
	test_breakpoint_two_threads();
	return 0;
}
