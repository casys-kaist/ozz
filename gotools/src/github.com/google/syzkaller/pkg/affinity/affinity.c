#define _GNU_SOURCE

#include <sched.h>

#include "affinity.h"

unsigned int get_affinity()
{
	cpu_set_t set;
	unsigned int mask = 0;
	if (sched_getaffinity(0, sizeof(set), &set))
		return 0;

#define NR_BITS 32
	for (int i = 0; i < NR_BITS; i++)
		if (CPU_ISSET(i, &set))
			mask |= 1 << i;

	return mask;
}
