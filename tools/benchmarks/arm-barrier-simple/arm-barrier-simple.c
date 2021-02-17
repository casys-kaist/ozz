#include <stdio.h>
#include <stdlib.h>

unsigned int *ptr1, *ptr2;

#define mb()            asm volatile("dmb ish" ::: "memory")
#define wmb()           asm volatile("dmb ishst" ::: "memory")
#define rmb()           asm volatile("dmb ishld" ::: "memory")

__attribute__((softstorebuffer))
int main()
{
	ptr1 = (unsigned int *)malloc(sizeof(*ptr1));
	ptr2 = (unsigned int *)malloc(sizeof(*ptr2));

	*ptr1 = 0xdeadbeaf;
	wmb();
	*ptr2 = 0xc0ffee;

	return 0;
}
