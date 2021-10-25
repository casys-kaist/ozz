#ifndef __HYPERCALL_H
#define __HYPERCALL_H

#define HCALL_RAX_ID 0x1d08aa3e
#define HCALL_INSTALL_BP 0xf477909a
#define HCALL_ACTIVATE_BP 0x40ab903
#define HCALL_DEACTIVATE_BP 0xf327524f
#define HCALL_CLEAR_BP 0xba220681
#define HCALL_ENABLE_KSSB 0x3dcb4536
#define HCALL_DISABLE_KSSB 0xbed348f5
#define HCALL_PREPARE_BP 0x23564d5a

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

#endif /* __HYPERCALL_H */
