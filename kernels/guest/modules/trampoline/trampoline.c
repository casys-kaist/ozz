#define pr_fmt(fmt) KBUILD_MODNAME ": " fmt

#include <linux/init.h>
#include <linux/module.h>
#include <linux/kernel.h>

#include "hcall.h"

MODULE_LICENSE("GPL");
MODULE_AUTHOR("Dae R. Jeong");
MODULE_DESCRIPTION("Trampoline module");
MODULE_VERSION("0.01");

static void __trampoline(void)
{
	// Do here whatever if needed
}

void trampoline(void)
{
	// Do nothing here
loop:
	__trampoline();
	goto loop;
}
EXPORT_SYMBOL(trampoline);

static int __init trampoline_init(void) {
	unsigned long addr = (unsigned long)trampoline;
	pr_info("Installing trampoline\n");
	pr_info("Trampoline addr: %lx\n", addr);
	hypercall(HCALL_VMI_FUNC_ADDR, VMI_TRAMPOLINE, addr, 0);
	return 0;
}

static void __exit trampoline_exit(void) {
	pr_info("Uninstalling trampoline\n");
}

module_init(trampoline_init);
module_exit(trampoline_exit);
