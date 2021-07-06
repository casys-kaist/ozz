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

#define HCALL_RAX_ID 0x1d08aa3e
#define HCALL_INSTALL_BP 0xf477909a
#define HCALL_ACTIVATE_BP 0x40ab903
#define HCALL_DEACTIVATE_BP 0xf327524f
#define HCALL_CLEAR_BP 0xba220681

unsigned long hypercall(unsigned long cmd, unsigned long arg,
						unsigned long subarg, unsigned long subarg2)
{
	unsigned long ret = -1;
	printf("Calling hypercall: %lx\n", cmd);
	fflush(stdout);
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
				 "addq $40, %%rsp\n\t"
				 "popq %%rbx\n\t"
				 : "=r"(ret)
				 : "r"(id), "r"(cmd), "r"(arg), "r"(subarg), "r"(subarg2));
#endif
	printf("Return: %lx\n", ret);
	fflush(stdout);
	return ret;
}

unsigned long sys_test_addr(void) {
	char buf[128];
	FILE *fp = popen("grep '__do_sys_ssb_pso_writer' /proc/kallsyms | head -n 1 | cut -d' ' -f1", "r");
	fgets(buf, sizeof(buf), fp);
	pclose(fp);
	return strtoul(buf, NULL, 16);
}

#define SYS_pso_writer 501
#define gettid() syscall(SYS_gettid)

void *thr(void *arg) {
	int cpu = (int)(intptr_t)arg;
	cpu_set_t set;
	CPU_ZERO(&set);
	CPU_SET(cpu, &set);
	if (sched_setaffinity(gettid(), sizeof(set), &set))
		perror("sched_setaffinity");
	hypercall(HCALL_INSTALL_BP, sys_test_addr(), cpu, 0);
	sleep(1);
	hypercall(HCALL_ACTIVATE_BP, 0, 0, 0);
	syscall(SYS_pso_writer);
	hypercall(HCALL_DEACTIVATE_BP, 0, 0, 0);
	hypercall(HCALL_CLEAR_BP, 0, 0, 0);
}

void test_single_thread(void) {
	fprintf(stderr, "%s\n", __func__);
	thr((void *)0);
}

void test_two_threads(void) {
	pthread_t pth1, pth2;

	fprintf(stderr, "%s\n", __func__);

	pthread_create(&pth1, NULL, thr, (void *)0);
	pthread_create(&pth2, NULL, thr, (void *)1);

	pthread_join(pth1, NULL);
	pthread_join(pth2, NULL);
}

int main(int argc, char *argv[])
{
	test_single_thread();
	test_two_threads();
	return 0;
}
