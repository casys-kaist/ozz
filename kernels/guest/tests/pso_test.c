#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif

#include <unistd.h>
#include <stdio.h>
#include <pthread.h>

#define SYS_SSB_FEEDINPUT 500
#define SYS_PSO_WRITER 501
#define SYS_PSO_READER 502

void *th1(void *_arg)
{
	int go = (int)(intptr_t)_arg;
	while(!go);
	syscall(SYS_PSO_WRITER);
	return NULL;
}

void *th2(void *_arg)
{
	int go = (int)(intptr_t)_arg;
	while(!go);
	syscall(SYS_PSO_READER);
	return NULL;
}

int main(void)
{
	pthread_t pth1, pth2;
	int go = 0;
	int flush_vector[] = {0, 1};

	syscall(SYS_SSB_FEEDINPUT, flush_vector, 2);

	pthread_create(&pth1, NULL, th1, (void *)&go);
	pthread_create(&pth2, NULL, th2, (void *)&go);

	go = 1;

	pthread_join(pth1, NULL);
	pthread_join(pth2, NULL);
	return 0;
}