#include <fcntl.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <sys/ioctl.h>
#include <sys/mman.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <unistd.h>

#include "hypercall.h"

enum kmemcov_access_type {
  KMEMCOV_ACCESS_STORE,
  KMEMCOV_ACCESS_LOAD,
  KMEMCOV_ACCESS_FLUSH,
  KMEMCOV_ACCESS_TOTAL,
};

struct kmemcov_access {
  unsigned long inst;
  unsigned long addr;
  size_t size;
  enum kmemcov_access_type type;
  uint64_t timestamp;
};

#define KMEMCOV_INIT_TRACE _IO('d', 100)
#define KMEMCOV_ENABLE _IO('d', 101)
#define KMEMCOV_DISABLE _IO('d', 102)
#define COVER_SIZE (64 << 10)

#define SYS_PSO_WRITER 501
#define SYS_PSO_READER 502
#define SYS_PSO_CLEAR 504

struct kmemcov_access *cover;

void run_writer(void) { syscall(SYS_PSO_WRITER, 1); }

void run_reader(void) { syscall(SYS_PSO_READER, 1); }

void run(void (*fn)(void)) {
  int n, i;
  /* Reset coverage from the tail of the ioctl() call. */
  __atomic_store_n((unsigned long *)&cover[0], 0, __ATOMIC_RELAXED);
  /* That's the target syscal call. */
  fn();
  /* Read number of PCs collected. */
  n = __atomic_load_n((unsigned long *)&cover[0], __ATOMIC_RELAXED);
  printf("%d\n", n);

  char c[KMEMCOV_ACCESS_TOTAL] = {
      'W',
      'R',
      'F',
  };
  for (i = 0; i < n; i++)
    printf("0x%lx    0x%lx    %c\n", cover[i + 1].inst, cover[i + 1].addr,
           c[cover[i + 1].type]);
}

int main(int argc, char **argv) {
  int fd;

  syscall(SYS_PSO_CLEAR);

  hypercall(HCALL_ENABLE_KSSB, 0, 0, 0);

  /* A single fd descriptor allows coverage collection on a single
   * thread.
   */
  fd = open("/sys/kernel/debug/kmemcov", O_RDWR);
  if (fd == -1)
    perror("open"), exit(1);
  /* Setup trace mode and trace size. */
  if (ioctl(fd, KMEMCOV_INIT_TRACE, COVER_SIZE))
    perror("ioctl"), exit(1);
  /* Mmap buffer shared between kernel- and user-space. */
  cover = (struct kmemcov_access *)mmap(
      NULL, COVER_SIZE * sizeof(struct kmemcov_access), PROT_READ | PROT_WRITE,
      MAP_SHARED, fd, 0);
  if ((void *)cover == MAP_FAILED)
    perror("mmap"), exit(1);
  /* Enable coverage collection on the current thread. */
  if (ioctl(fd, KMEMCOV_ENABLE, 0))
    perror("ioctl"), exit(1);
  /* Collect memory accesses */
  run(run_writer);
  run(run_reader);
  /* Disable coverage collection for the current thread. After this call
   * coverage can be enabled for a different thread.
   */
  if (ioctl(fd, KMEMCOV_DISABLE, 0))
    perror("ioctl"), exit(1);
  /* Free resources. */
  if (munmap(cover, COVER_SIZE * sizeof(struct kmemcov_access)))
    perror("munmap"), exit(1);
  if (close(fd))
    perror("close"), exit(1);
  return 0;
}
