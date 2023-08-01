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
  hypercall(HCALL_INSTALL_BP, 0xffffffff81b6ade2, 0, 0);
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

int main(void) {
  pin(0);
  do_test(true);
  return 0;
}
