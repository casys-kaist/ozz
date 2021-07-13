#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif

#include <unistd.h>
#include <stdio.h>
#include <pthread.h>
#include <sys/types.h>
#include <syscall.h>
#include <stdlib.h>

#define SYS_SSB_FEEDINPUT 500
#define SYS_PSO_WRITER 501
#define SYS_PSO_READER 502
#define SYS_PSO_CLEAR 504

#define HCALL_RAX_ID 0x1d08aa3e
#define HCALL_INSTALL_BP 0xf477909a
#define HCALL_ACTIVATE_BP 0x40ab903
#define HCALL_DEACTIVATE_BP 0xf327524f
#define HCALL_CLEAR_BP 0xba220681

#define gettid() syscall(SYS_gettid)

unsigned long hypercall(unsigned long cmd, unsigned long arg,
						unsigned long subarg, unsigned long subarg2)
{
	unsigned long ret = -1;
#ifdef __amd64__
	unsigned long id = HCALL_RAX_ID;
	asm volatile(
				 // rbx is a callee-saved register
				 "pushq %%rbx\n\t"
				 // Save values to the stack, so below movqs always
				 // see consistent values.
				 "pushq %1\n\t"
				 "pushq %2\n\t"
				 "pushq %3\n\t"
				 "pushq %4\n\t"
				 "pushq %5\n\t"
				 // Setup registers
				 "movq 32(%%rsp), %%rax\n\t"
				 "movq 24(%%rsp), %%rbx\n\t"
				 "movq 16(%%rsp), %%rcx\n\t"
				 "movq 8(%%rsp), %%rdx\n\t"
				 "movq (%%rsp), %%rsi\n\t"
				 // then vmcall
				 "vmcall\n\t"
				 // clear the stack
				 "addq $40,%%rsp\n\t"
				 "popq %%rbx\n\t"
				 : "=r"(ret)
				 : "r"(id), "r"(cmd), "r"(arg), "r"(subarg), "r"(subarg2));
#endif
	return ret;
}

unsigned long breakpoint_addr(void) {
	char buf[128];
	FILE *fp = popen("grep 'pso_test_breakpoint' /proc/kallsyms | head -n 1 | cut -d' ' -f1", "r");
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

	hypercall(HCALL_INSTALL_BP, breakpoint_addr(), cpu-1, 0);

	while(!*go);

	syscall(SYS_PSO_WRITER, 0);
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

	syscall(SYS_PSO_READER, 0);
	hypercall(HCALL_DEACTIVATE_BP, 0, 0, 0);
	hypercall(HCALL_CLEAR_BP, 0, 0, 0);
	return NULL;
}

int main(void)
{
	int cpu = 0;
	cpu_set_t set;
	pthread_t pth1, pth2;
	int go = 0;
	int flush_vector[] = {1, 0};

	CPU_ZERO(&set);
	CPU_SET(cpu, &set);
	if (sched_setaffinity(gettid(), sizeof(set), &set))
		perror("sched_setaffinity");

	syscall(SYS_SSB_FEEDINPUT, flush_vector, 2);

	pthread_create(&pth1, NULL, th1, (void *)&go);
	pthread_create(&pth2, NULL, th2, (void *)&go);

	sleep(3);
	hypercall(HCALL_ACTIVATE_BP, 0, 0, 0);
	go = 1;

	pthread_join(pth1, NULL);
	pthread_join(pth2, NULL);

	syscall(SYS_PSO_CLEAR);

	return 0;
}
