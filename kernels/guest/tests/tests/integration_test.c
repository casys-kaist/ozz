#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif

#include <unistd.h>
#include <stdio.h>
#include <pthread.h>
#include <sys/types.h>
#include <syscall.h>
#include <stdlib.h>

#include "hypercall.h"

#define SYS_SSB_FEEDINPUT 500
#define SYS_PSO_WRITER 501
#define SYS_PSO_READER 502
#define SYS_PSO_CLEAR 504

#define EINVAL 22

#define gettid() syscall(SYS_gettid)

unsigned long breakpoint_addr;
unsigned long get_breakpoint_addr(void) {
	char buf[128];
	FILE *fp = popen("grep 'kssb_test_breakpoint' /proc/kallsyms | head -n 1 | cut -d' ' -f1", "r");
	fgets(buf, sizeof(buf), fp);
	pclose(fp);
	return strtoul(buf, NULL, 16);
}

void *th1(void *_arg)
{
	int *go = (int *)_arg;
	int cpu = 1;
	cpu_set_t set;

	CPU_ZERO(&set);
	CPU_SET(cpu, &set);

	if (sched_setaffinity(gettid(), sizeof(set), &set))
		perror("sched_setaffinity");

	hypercall(HCALL_INSTALL_BP, breakpoint_addr, cpu-1, 0);

	while(!*go);

	syscall(SYS_PSO_WRITER, 0, 0);
	hypercall(HCALL_DEACTIVATE_BP, 0, 0, 0);
	hypercall(HCALL_CLEAR_BP, 0, 0, 0);
	return NULL;
}

void *th2(void *_arg)
{
	int *go = (int *)_arg;
	int cpu = 2;
	cpu_set_t set;

	CPU_ZERO(&set);
	CPU_SET(cpu, &set);

	if (sched_setaffinity(gettid(), sizeof(set), &set))
		perror("sched_setaffinity");

	hypercall(HCALL_INSTALL_BP, 0xffffffffffffffff, cpu-1, 0);

	while(!*go);

	syscall(SYS_PSO_READER, 0, 0);
	hypercall(HCALL_DEACTIVATE_BP, 0, 0, 0);
	hypercall(HCALL_CLEAR_BP, 0, 0, 0);
	return NULL;
}

void do_test(void)
{
	int cpu = 0, go = 0, cnt = 5;
	cpu_set_t set;
	pthread_t pth1, pth2;
	int flush_vector[] = {1, 0};
	unsigned long hcall_ret;

	syscall(SYS_PSO_CLEAR);

	CPU_ZERO(&set);
	CPU_SET(cpu, &set);
	if (sched_setaffinity(gettid(), sizeof(set), &set))
		perror("sched_setaffinity");

	hypercall(HCALL_PREPARE_BP, 2, 0, 0);

	syscall(SYS_SSB_FEEDINPUT, flush_vector, 2);

	pthread_create(&pth1, NULL, th1, (void *)&go);
	pthread_create(&pth2, NULL, th2, (void *)&go);

	do {
		hcall_ret = hypercall(HCALL_ACTIVATE_BP, 0, 0, 0);
		usleep(100 * 1000);
	} while(hcall_ret == -EINVAL && --cnt);

	go = 1;

	pthread_join(pth1, NULL);
	pthread_join(pth2, NULL);
}

int main(void)
{
	breakpoint_addr = get_breakpoint_addr();
	do_test();
	fprintf(stderr, "The kernel should not panic.\n");
	hypercall(HCALL_ENABLE_KSSB, 0, 0, 0);
	do_test();
	fprintf(stderr, "The kernel should panic.\n");
	return 0;
}