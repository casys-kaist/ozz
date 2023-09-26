#ifndef __TEST_H
#define __TEST_H

#include <sched.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>

#include "hypercall.h"

#define SYS_SSB_FEEDINPUT 500
#define SYS_SSB_SWITCH 503

#define EINVAL 22
#define EAGAIN 11

bool do_sleep() {
  for (int i = 0; i < 10000000; i++)
    ;
  return true;
}

void activate_bp_sync(void) {
  unsigned long ret;
  do {
    ret = hypercall(HCALL_ACTIVATE_BP, 0, 0, 0);
    if (ret == -EINVAL)
      exit(-1);
  } while (ret == -EAGAIN && do_sleep());
}

struct vec_t {
  int size;
  int vec[4];
} flush_vectors[] = {
    {2, {1, 0}},       {2, {0, 1}},       {3, {1, 1, 0}},    {3, {1, 0, 1}},
    {3, {0, 1, 1}},    {3, {1, 0, 0}},    {3, {0, 1, 0}},    {3, {0, 0, 1}},
    {4, {0, 0, 0, 1}}, {4, {0, 0, 1, 0}}, {4, {0, 1, 0, 0}}, {4, {1, 0, 0, 0}},
    {4, {0, 0, 1, 1}}, {4, {0, 1, 0, 1}}, {4, {1, 0, 0, 1}}, {4, {0, 1, 1, 0}},
    {4, {1, 1, 0, 0}}, {4, {1, 0, 1, 0}}, {4, {0, 1, 1, 1}}, {4, {1, 0, 1, 1}},
    {4, {1, 1, 0, 1}}, {4, {1, 1, 1, 0}},
};

void run();

void do_test() {
  for (int i = 0; i < sizeof(flush_vectors) / sizeof(flush_vectors[0]); i++) {
    struct vec_t *v = &flush_vectors[i];
    printf("Flush vector:\n");
    for (int j = 0; j < v->size; j++)
      printf("%d ", v->vec[j]);
    printf("\n");
    syscall(SYS_SSB_FEEDINPUT, v->vec, v->size);
    run();
  }
}

void pin(int cpu) {
  cpu_set_t set;
  CPU_ZERO(&set);
  CPU_SET(cpu, &set);
  sched_setaffinity(0, sizeof(set), &set);
}

#endif
