#ifndef __TEST_H
#define __TEST_H

#include <stdbool.h>
#include <stdio.h>
#include <unistd.h>

#include "hypercall.h"

#define SYS_SSB_FEEDINPUT 500
#define SYS_SSB_SWITCH 503

struct vec_t {
  int size;
  int vec[3];
} flush_vectors[] = {
    {2, {1, 0}},    {2, {0, 1}},    {3, {1, 1, 0}}, {3, {1, 0, 1}},
    {3, {0, 1, 1}}, {3, {1, 0, 0}}, {3, {0, 1, 0}}, {3, {0, 0, 1}},
};

void run();

void do_test(bool enable_kssb) {
  if (enable_kssb)
    hypercall(HCALL_ENABLE_KSSB, 0, 0, 0);
  for (int i = 0; i < sizeof(flush_vectors) / sizeof(flush_vectors[0]); i++) {
    struct vec_t *v = &flush_vectors[i];
    printf("Flush vector:\n");
    for (int j = 0; j < v->size; j++)
      printf("%d ", v->vec[j]);
    printf("\n");
    syscall(SYS_SSB_FEEDINPUT, v->vec, v->size);
    run();
  }
  if (enable_kssb)
    hypercall(HCALL_DISABLE_KSSB, 0, 0, 0);
}

#endif
