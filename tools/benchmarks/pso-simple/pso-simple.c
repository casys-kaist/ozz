#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif

#include <stdlib.h>
#include <pthread.h>
#include <stdbool.h>
#include <stdint.h>
#include <unistd.h>

struct shared_t {
	int *ptr;
	bool ready;
};

__attribute__((softstorebuffer))
void *writer(void *_arg)
{
	struct shared_t *arg = (struct shared_t *)_arg;
	arg->ptr = (int *)malloc(sizeof(*arg->ptr));
	arg->ready = true;
	sleep(3);
	return NULL;
}

__attribute__((softstorebuffer))
void *reader(void *_arg)
{
	struct shared_t *arg = (struct shared_t *)_arg;
	sleep(1);
	if (arg->ready)
		(void)*arg->ptr;
	return NULL;
}

#define _FLUSH
#ifdef _FLUSH
extern void __ssb_pso_feedinput(uint32_t vector[], size_t size);
#endif

__attribute__((no_softstorebuffer))
int main(int argc, char *argv[])
{
#ifdef _FLUSH
	// Suppose a fuzzer provides an input that do not flush arg->ptr
	// but flush arg->ready.
	uint32_t input[2] = { 0, 1 };
	__ssb_pso_feedinput(input, 2);
#endif
	pthread_t pth1, pth2;
	struct shared_t shared = { 0, 0 };

	pthread_create(&pth2, NULL, writer, &shared);
	pthread_create(&pth1, NULL, reader, &shared);

	pthread_join(pth1, NULL);
	pthread_join(pth2, NULL);

	return 0;
}
