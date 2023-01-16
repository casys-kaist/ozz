#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif

#include <pthread.h>
#include <stdio.h>
#include <unistd.h>

#include "hypercall.h"

#define SYS_SSB_FEEDINPUT 500
#define SYS_PSO_WRITER 501
#define SYS_PSO_READER 502
#define SYS_SSB_SWITCH 503
#define SYS_PSO_CLEAR 504

void *th1(void *_arg) {
  int go = (int)(intptr_t)_arg;
  syscall(SYS_SSB_SWITCH);
  while (!go)
    ;
  syscall(SYS_PSO_WRITER, 1, 1);
  return NULL;
}

void *th2(void *_arg) {
  int go = (int)(intptr_t)_arg;
  syscall(SYS_SSB_SWITCH);
  while (!go)
    ;
  syscall(SYS_PSO_READER, 1, 1);
  return NULL;
}

void do_test(void) {
  pthread_t pth1, pth2;
  int go = 0;
  int flush_vector[] = {1, 0};

  syscall(SYS_PSO_CLEAR);

  syscall(SYS_SSB_FEEDINPUT, flush_vector, 2);

  pthread_create(&pth1, NULL, th1, (void *)&go);
  pthread_create(&pth2, NULL, th2, (void *)&go);

  go = 1;

  pthread_join(pth1, NULL);
  pthread_join(pth2, NULL);
}

int main(void) {
  do_test();
  fprintf(stderr, "The kernel should not panic.\n");
  hypercall(HCALL_ENABLE_KSSB, 0, 0, 0);
  do_test();
  fprintf(stderr, "The kernel should panic here.\n");
  return 0;
}
