#define _GNU_SOURCE

#include <pthread.h>
#include <stdbool.h>
#include <stdint.h>
#include <assert.h>

struct shared_t {
	int a;
	int b;
};

void *thr1(void *_arg)
{
	struct shared_t *arg = _arg;
	arg->b = 1;
	int la = arg->a;
	return (void *)(intptr_t)la;
}

void *thr2(void *_arg)
{
	struct shared_t *arg = _arg;
	arg->a = 1;
	int lb = arg->b;
	return (void *)(intptr_t)lb;
}

int main(int argc, char *argv[])
{
	pthread_t pth1, pth2;
	struct shared_t shared = { 0, 0 };
	int ret1, ret2;

	pthread_create(&pth1, NULL, thr1, &shared);
	pthread_create(&pth2, NULL, thr2, &shared);

	pthread_join(pth1, (void *)&ret1);
	pthread_join(pth2, (void *)&ret2);

	assert(ret1 || ret2);

	return 0;
}
