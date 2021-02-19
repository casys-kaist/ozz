#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif

#include <pthread.h>
#include <stdbool.h>
#include <stdint.h>
#include <assert.h>
#include <unistd.h>

struct shared_t {
	int a;
	int b;
};

__attribute__((softstorebuffer))
void *thr1(void *_arg)
{
	struct shared_t *arg = (struct shared_t *)_arg;
	arg->b = 1;
	int la = arg->a;
	sleep(1);
	return (void *)(intptr_t)la;
}

__attribute__((softstorebuffer))
void *thr2(void *_arg)
{
	struct shared_t *arg = (struct shared_t *)_arg;
	arg->a = 1;
	int lb = arg->b;
	sleep(1);
	return (void *)(intptr_t)lb;
}

#define _FLUSH
#ifdef _FLUSH
extern void __ssb_tso_feedinput(uint32_t vector[], size_t size);
#endif

int main(int argc, char *argv[])
{
	// Suppose a fuzzer provides an input that flush all storebuffer
	// entries immediately after a store.
#ifdef _FLUSH
	uint32_t input[1] = { 1 };
	__ssb_tso_feedinput(input, 1);
#endif

	pthread_t pth1, pth2;
	struct shared_t shared = { 0, 0 };
	int ret1, ret2;

	pthread_create(&pth1, NULL, thr1, &shared);
	pthread_create(&pth2, NULL, thr2, &shared);

	pthread_join(pth1, (void **)&ret1);
	pthread_join(pth2, (void **)&ret2);

	assert(ret1 || ret2);

	return 0;
}
