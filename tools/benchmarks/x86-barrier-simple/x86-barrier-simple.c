#include <stdio.h>
#include <stdlib.h>

#define mb()    asm volatile("mfence" ::: "memory")
#define rmb()   asm volatile("lfence" ::: "memory")
#define wmb()   asm volatile("sfence" ::: "memory")

unsigned int *ptr1, *ptr2;

__attribute__((softstorebuffer))
int main(int argc, char *argv[])
{
	unsigned int local;
	// Prepare variables
	ptr1 = (unsigned int *)malloc(sizeof(*ptr1));
	ptr2 = (unsigned int *)malloc(sizeof(*ptr2));
	*ptr2 = 0xc0ffee;
	// Order store/load instructions
	*ptr1 = 0xdeadbeaf;  // store
	wmb(); 	             // prevent store-load reordering
	local = *ptr2;       // load
	return 0;
}
