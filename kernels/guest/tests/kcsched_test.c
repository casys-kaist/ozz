#define _GNU_SOURCE

#include <stdio.h>
#include <unistd.h>
#include <stdlib.h>
#include <errno.h>

#define sys_setup_schedule 505
#define sys_clear_schedule 506
#define sys_freeze_schedule 507
#define sys_unfreeze_schedule 508

struct kcschedpoint {
	unsigned long addr;
	int order;
} scheds[3] = {
	{0xabcd, 0},
	{0x1234, 1},
	{0xdeadbeaf, 2}
};

void test_setup_and_clear_schedule(void)
{
	printf("%s\n", __func__);
	if(syscall(sys_setup_schedule, 1, 3, scheds))
		perror("setup_schedule"), exit(1);

	if(syscall(sys_clear_schedule, 1))
		perror("clear_schedule"), exit(1);
}

void test_setup_and_clear_twice_schedule(void) {
	printf("%s\n", __func__);
	test_setup_and_clear_schedule();
	test_setup_and_clear_schedule();
}

void test_setup_and_freeze_and_unfreeze_and_clear_schedule(void) {
	printf("%s\n", __func__);
	if(syscall(sys_setup_schedule, 1, 3, scheds))
		perror("setup_schedule"), exit(1);

	if(syscall(sys_freeze_schedule, 1, 1))
		perror("freeze_schedule"), exit(1);

	if(syscall(sys_unfreeze_schedule, 1, 1))
		perror("unfreeze_schedule"), exit(1);

	if(syscall(sys_clear_schedule, 1))
		perror("clear_schedule"), exit(1);
}

int main(int arch, char *argv[])
{
	/* Benign behaviors */
	test_setup_and_clear_schedule();
	test_setup_and_clear_twice_schedule();
	test_setup_and_freeze_and_unfreeze_and_clear_schedule();
	return 0;
}
