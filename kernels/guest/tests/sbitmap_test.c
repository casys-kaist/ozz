#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif

#include <unistd.h>
#include <stdio.h>
#include <pthread.h>

#define SYS_SSB_FEEDINPUT 500
#define SYS_SBITMAP_INIT 505
#define SYS_SBITMAP_FUNC1 506
#define SYS_SBITMAP_FUNC2 507
#define SYS_SBITMAP_CLEAR 508

void *th1(void *_arg)
{
	int go;

	while(1){
		syscall(SYS_SBITMAP_FUNC1);
		go = *(int *)_arg;
		if(!go) break;
	}

	return NULL;
}

void *th2(void *_arg)
{
	int go;

	while(1) {
		syscall(SYS_SBITMAP_FUNC2);
		go = *(int *)_arg;
		if(!go) break;
	}

	return NULL;
}

int main(void)
{
	pthread_t pth1, pth2;
	int go = 0;
	int flush_vector[] = {1, 0};

	syscall(SYS_SSB_FEEDINPUT, flush_vector, 2);
	syscall(SYS_SBITMAP_INIT);

	pthread_create(&pth1, NULL, th1, (void *)&go);
	pthread_create(&pth2, NULL, th2, (void *)&go);

	go = 1;

	sleep(1200);

	go = 0;
	pthread_join(pth1, NULL);
	pthread_join(pth2, NULL);

	syscall(SYS_SBITMAP_CLEAR);

	return 0;
}
