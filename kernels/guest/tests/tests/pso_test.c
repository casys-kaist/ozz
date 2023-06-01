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

void run(int *flush_vector, int size) {
  pthread_t pth1, pth2;
  int go = 0;

  syscall(SYS_PSO_CLEAR);
  syscall(SYS_SSB_FEEDINPUT, flush_vector, size);

  pthread_create(&pth1, NULL, th1, (void *)&go);
  pthread_create(&pth2, NULL, th2, (void *)&go);

  go = 1;

  pthread_join(pth1, NULL);
  pthread_join(pth2, NULL);
}

void do_test(void) {
  struct vec_t {
    int size;
    int vec[3];
  } flush_vectors[] = {
      {2, {1, 0}},    {2, {0, 1}},    {3, {1, 1, 0}}, {3, {1, 0, 1}},
      {3, {0, 1, 1}}, {3, {1, 0, 0}}, {3, {0, 1, 0}}, {3, {0, 0, 1}},
  };

  for (int i = 0; i < sizeof(flush_vectors) / sizeof(flush_vectors[0]); i++) {
    struct vec_t *v = &flush_vectors[i];
    printf("Flush vector:\n");
    for (int j = 0; j < v->size; j++)
      printf("%d ", v->vec[j]);
    printf("\n");
    run(v->vec, v->size);
  }
}

int main(void) {
  do_test();
  fprintf(stderr, "The kernel should not panic.\n");
  hypercall(HCALL_ENABLE_KSSB, 0, 0, 0);
  do_test();
  fprintf(stderr, "The kernel should panic here.\n");
  return 0;
}
