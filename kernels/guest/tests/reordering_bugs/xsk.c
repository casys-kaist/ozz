#define _GNU_SOURCE

#include <errno.h>
#include <pthread.h>
#include <stdio.h>
#include <sys/mman.h>
#include <sys/socket.h>
#include <sys/types.h>

#include "test.h"

#define AF_XDP 44
#define XDP_UMEM_PGOFF_FILL_RING 0x100000000ULL
#define FQ_NUM_DESCS 1024
#define XDP_UMEM_FILL_RING 5
#define SOL_XDP 283

int sk;

void *th1(void *unused) {
  pin(1);

  hypercall(HCALL_INSTALL_BP, 0xffffffff8f34f2c4, 0, 0);

  activate_bp_sync();

  syscall(SYS_SSB_SWITCH);
  int fq_size = FQ_NUM_DESCS;
  if (setsockopt(sk, SOL_XDP, XDP_UMEM_FILL_RING, &fq_size, sizeof(int)))
    perror("setsockopt");
  hypercall(HCALL_DEACTIVATE_BP, 0, 0, 0);
}

void *th2(void *unused) {
  pin(2);

  hypercall(HCALL_INSTALL_BP, 0xffffffffffffffff, 1, 0);

  activate_bp_sync();

  syscall(SYS_SSB_SWITCH);
  if (mmap(0, 0x1000, PROT_READ | PROT_WRITE, MAP_SHARED | MAP_POPULATE, sk,
           XDP_UMEM_PGOFF_FILL_RING) == MAP_FAILED)
    printf("mmap: %d\n", errno);
  hypercall(HCALL_DEACTIVATE_BP, 0, 0, 0);
}

void run() {
  hypercall(HCALL_RESET, 0, 0, 0);
  hypercall(HCALL_PREPARE, 2, 2, 0);

  hypercall(HCALL_ENABLE_KSSB, 0, 0, 0);

  sk = socket(AF_XDP, SOCK_RAW, 0);

  pthread_t pth1, pth2;

  pthread_create(&pth1, NULL, th1, NULL);
  pthread_create(&pth2, NULL, th2, NULL);

  pthread_join(pth1, NULL);
  pthread_join(pth2, NULL);

  close(sk);

  hypercall(HCALL_DISABLE_KSSB, 0, 0, 0);

  hypercall(HCALL_RESET, 0, 0, 0);
}

int main() {
  pin(0);
  do_test(true);
  return 0;
}
