#define pr_fmt(fmt) KBUILD_MODNAME ": " fmt

#include <linux/init.h>
#include <linux/module.h>
#include <linux/kernel.h>
#include <linux/kallsyms.h>

#include "hcall.h"

MODULE_LICENSE("GPL");
MODULE_AUTHOR("Dae R. Jeong");
MODULE_DESCRIPTION("VMI helper module");
MODULE_VERSION("0.01");

static int __init vmihelper_init(void) {
	char *hook_name = "qcsched_hook_entry";
	unsigned long addr = kallsyms_lookup_name(hook_name);
	unsigned long ret;

	pr_info("Installing vmihelper\n");
	pr_info("hook addr: %lx\n", addr);

	if (addr == 0) {
		pr_info("failed to get the hook address\n");
	} else {
		ret = hypercall(HCALL_VMI_FUNC_ADDR, VMI_HOOK, addr, 0);
		pr_info("return: %lx\n", ret);
	}

	return 0;
}

static void __exit vmihelper_exit(void) {
	pr_info("Uninstalling vmihelper\n");
}

module_init(vmihelper_init);
module_exit(vmihelper_exit);