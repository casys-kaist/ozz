#define _GNU_SOURCE

#include <errno.h>
#include <pthread.h>
#include <sched.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <sys/types.h>
#include <syscall.h>
#include <unistd.h>

#include "hypercall.h"

unsigned long sys_test_addr;
unsigned long get_sys_test_addr(void) {
  char buf[128];
  FILE *fp = popen("grep '__x64_sys_ssb_pso_writer' /proc/kallsyms | head -n 1 "
                   "| cut -d' ' -f1",
                   "r");
  fgets(buf, sizeof(buf), fp);
  pclose(fp);
  return strtoul(buf, NULL, 16);
}

#define SYS_pso_writer 501
#define gettid() syscall(SYS_gettid)

struct arg_t {
  int cpu;
  bool *go;
};

void *thr(void *_arg) {
  struct arg_t *arg = (struct arg_t *)_arg;
  int cpu = arg->cpu;
  cpu_set_t set;
  CPU_ZERO(&set);
  CPU_SET(cpu, &set);
  if (sched_setaffinity(gettid(), sizeof(set), &set))
    perror("sched_setaffinity");
  hypercall(HCALL_INSTALL_BP, sys_test_addr, cpu, 0);
  while (!*arg->go)
    ;
  hypercall(HCALL_ACTIVATE_BP, 0, 0, 0);
  syscall(SYS_pso_writer, 1);
  hypercall(HCALL_DEACTIVATE_BP, 0, 0, 0);
  hypercall(HCALL_CLEAR_BP, 0, 0, 0);
}

void test_single_thread(void) {
  bool go = true;
  struct arg_t arg = {.cpu = 0, .go = &go};
  fprintf(stderr, "%s\n", __func__);
  hypercall(HCALL_PREPARE, 1, 0, 0);
  thr(&arg);
}

void test_two_threads(void) {
  bool go = false;
  pthread_t pth1, pth2;
  struct arg_t arg0 = {.cpu = 0, .go = &go};
  struct arg_t arg1 = {.cpu = 1, .go = &go};
  ;

  fprintf(stderr, "%s\n", __func__);

  hypercall(HCALL_PREPARE, 2, 0, 0);

  pthread_create(&pth1, NULL, thr, &arg0);
  pthread_create(&pth2, NULL, thr, &arg1);

  printf("---------------------\n");
  sleep(2);
  go = true;

  pthread_join(pth1, NULL);
  pthread_join(pth2, NULL);
}

void test_kssb_turn_on_off(void) {
  for (int i = 0; i < 10; i++) {
    hypercall(HCALL_ENABLE_KSSB, 0, 0, 0);
    sleep(1);
    hypercall(HCALL_DISABLE_KSSB, 0, 0, 0);
    sleep(1);
  }
}

int main(int argc, char *argv[]) {
  sys_test_addr = get_sys_test_addr();
  test_single_thread();
  test_two_threads();
  test_kssb_turn_on_off();
  return 0;
}
