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
				 "movq %1, %%rax\n\t"
				 "movq %2, %%rbx\n\t"
				 "movq %3, %%rcx\n\t"
				 "movq %4, %%rdx\n\t"
				 "movq %5, %%rsi\n\t"
				 "vmcall\n\t"
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

int main(int argc, char *argv[])
{
	cpu_set_t set;

	CPU_ZERO(&set);
	CPU_SET(0, &set);

	if (sched_setaffinity(gettid(), sizeof(set), &set))
		perror("sched_setaffinity");

	hypercall(HCALL_INSTALL_BP, sys_test_addr(), 0, 0);
	hypercall(HCALL_ACTIVATE_BP, 0, 0, 0);
	syscall(SYS_pso_writer);
	hypercall(HCALL_DEACTIVATE_BP, 0, 0, 0);
	hypercall(HCALL_CLEAR_BP, 0, 0, 0);

	return 0;
}
