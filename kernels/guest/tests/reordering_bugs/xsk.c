#include <pthread.h>
#include <stdio.h>
#include <sys/mman.h>
#include <sys/socket.h>
#include <sys/types.h>

#define AF_XDP 44
#define XDP_UMEM_PGOFF_FILL_RING 0x100000000ULL
#define FQ_NUM_DESCS 1024
#define XDP_UMEM_FILL_RING 5
#define SOL_XDP 283

int sk;

void *th1(void *unused) {
  int fq_size = FQ_NUM_DESCS;
  if (setsockopt(sk, SOL_XDP, XDP_UMEM_FILL_RING, &fq_size, sizeof(int)))
    perror("setsockopt");
}

void *th2(void *unused) {
  if (mmap(0, 0x1000, PROT_READ | PROT_WRITE, MAP_SHARED | MAP_POPULATE, sk,
           XDP_UMEM_PGOFF_FILL_RING) == MAP_FAILED)
    perror("mmap");
}

int main() {
  sk = socket(AF_XDP, SOCK_RAW, 0);
  printf("%d\n", sk);

  pthread_t pth1, pth2;

  pthread_create(&pth1, NULL, th1, NULL);
  pthread_create(&pth2, NULL, th2, NULL);

  pthread_join(pth1, NULL);
  pthread_join(pth2, NULL);

  return 0;
}
