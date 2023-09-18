#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif

#include <pthread.h>
#include <stdio.h>
#include <unistd.h>

#include "hypercall.h"
#include "test.h"

#define SYS_PSO_WRITER 501
#define SYS_PSO_READER 502
#define SYS_PSO_CLEAR 504

// without rmb
#define INST_STORE_VAL 0xffffffff81bad6e4
#define INST_STORE_FLAG 0xffffffff81bad797
#define INST_LOAD_VAL 0xffffffff81badafa
#define INST_LOAD_FLAG 0xffffffff81bada4c

void *th1(void *_arg) {
  pin(1);
  hypercall(HCALL_INSTALL_BP, INST_STORE_FLAG, 0, 0);
  activate_bp_sync();
  syscall(SYS_SSB_SWITCH);
  syscall(SYS_PSO_WRITER, 0, 0, 0);
  hypercall(HCALL_DEACTIVATE_BP, 0, 0, 0);
  return NULL;
}

void *th2(void *_arg) {
  pin(2);
  hypercall(HCALL_INSTALL_BP, 0xffffffffffffffff, 1, 0);
  activate_bp_sync();
  syscall(SYS_SSB_SWITCH);
  syscall(SYS_PSO_READER, 0, 0);
  hypercall(HCALL_DEACTIVATE_BP, 0, 0, 0);
  return NULL;
}

void run() {
  hypercall(HCALL_RESET, 0, 0, 0);
  hypercall(HCALL_PREPARE, 2, 2, 0);

  pthread_t pth1, pth2;

  syscall(SYS_PSO_CLEAR);

  pthread_create(&pth1, NULL, th1, NULL);
  pthread_create(&pth2, NULL, th2, NULL);

  pthread_join(pth1, NULL);
  pthread_join(pth2, NULL);

  hypercall(HCALL_RESET, 0, 0, 0);
}

struct kssb_flush_table_entry {
  unsigned long inst;
  int value;
  void *pad1, *pad2;
};

int main(void) {
  pin(0);
  hypercall(HCALL_ENABLE_KSSB, 0, 0, 0);
  int vec[1] = {1};
  struct kssb_flush_table_entry table[4] = {
      {INST_STORE_VAL, 0}, // reordered (buffer)
      {INST_STORE_FLAG, 1}, // not reordered
      {INST_LOAD_VAL, 0}, // reordered (prefetched)
      {INST_LOAD_FLAG, 1}, // not reordered
  };
  syscall(SYS_SSB_FEEDINPUT, &vec, 1, &table, 4);
  run();
  hypercall(HCALL_DISABLE_KSSB, 0, 0, 0);
  return 0;
}
