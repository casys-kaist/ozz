#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif

#include <stdlib.h>
#include <pthread.h>
#include <stdbool.h>

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
	return NULL;
}

__attribute__((softstorebuffer))
void *reader(void *_arg)
{
	struct shared_t *arg = (struct shared_t *)_arg;
	if (arg->ready)
		(void)*arg->ptr;
	return NULL;
}

__attribute__((no_softstorebuffer))
int main(int argc, char *argv[])
{
	pthread_t pth1, pth2;
	struct shared_t shared = { 0, 0 };

	pthread_create(&pth2, NULL, writer, &shared);
	pthread_create(&pth1, NULL, reader, &shared);

	pthread_join(pth1, NULL);
	pthread_join(pth2, NULL);

	return 0;
}
