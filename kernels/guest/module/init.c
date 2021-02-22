#include <linux/init.h>
#include <linux/module.h>
#include <linux/kernel.h>

MODULE_LICENSE("GPL");
MODULE_AUTHOR("Dae R. Jeong");
MODULE_DESCRIPTION("A simple example to test softstorebuffer");
MODULE_VERSION("0.01");

static int __init ssb_test_init(void) {
	return 0;
}
static void __exit ssb_test_exit(void) {
}

module_init(ssb_test_init);
module_exit(ssb_test_exit);
