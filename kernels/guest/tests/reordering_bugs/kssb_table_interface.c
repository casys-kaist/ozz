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

void *th1(void *_arg) {
  pin(1);
  hypercall(HCALL_INSTALL_BP, 0xffffffff81bd2db7, 0, 0);
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

  hypercall(HCALL_ENABLE_KSSB, 0, 0, 0);

  syscall(SYS_PSO_CLEAR);

  pthread_create(&pth1, NULL, th1, NULL);
  pthread_create(&pth2, NULL, th2, NULL);

  pthread_join(pth1, NULL);
  pthread_join(pth2, NULL);

  hypercall(HCALL_DISABLE_KSSB, 0, 0, 0);
  hypercall(HCALL_RESET, 0, 0, 0);
}

struct kssb_flush_table_entry {
  unsigned long inst;
  int value;
  void *pad1, *pad2;
};

int main(void) {
  pin(0);
  int vec[1] = {1};
  struct kssb_flush_table_entry table[2] = {
      {0xffffffff81bd2d8e, 0},
      {0xffffffff81bd2db7, 1},
  };
  syscall(SYS_SSB_FEEDINPUT, &vec, 1, &table, 2);
  run();
  return 0;
}
