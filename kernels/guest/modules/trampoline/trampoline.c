#include <linux/init.h>
#include <linux/module.h>
#include <linux/kernel.h>

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
	printk(KERN_INFO "Installing trampoline\n");
	return 0;
}

static void __exit trampoline_exit(void) {
	printk(KERN_INFO "Uninstalling trampoline\n");
}

module_init(trampoline_init);
module_exit(trampoline_exit);
