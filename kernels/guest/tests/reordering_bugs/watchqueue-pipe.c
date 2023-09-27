// Fixed by 2ed147f015af2b48f41c6f0b6746aa9ea85c19f3

/* r0 = add_key(&(0x7f0000000200)='cifs.spnego\x00', &(0x7f00000001c0)={'syz',
 * 0x2}, 0x0, 0x0, 0xfffffffffffffffc) */
/* pipe2$watch_queue(&(0x7f0000001b80)={<r1=>0xffffffffffffffff,
 * <r2=>0xffffffffffffffff}, 0x80) */
/* keyctl$KEYCTL_WATCH_KEY(0x20, r0, r2, 0x0) */
/* read$FUSE(r1, &(0x7f0000001bc0)={0x2020}, 0x2020) */
/* ioctl$IOC_WATCH_QUEUE_SET_SIZE(r1, 0x5760, 0x10) */
/* keyctl$KEYCTL_WATCH_KEY(0xf, r0, 0xffffffffffffffff, 0x0) */

/* pipe2(&(0x7f0000000000)={<r0=>0xffffffffffffffff, <r1=>0xffffffffffffffff},
 * 0x4000) */
/* read$FUSE(r0, &(0x7f0000000500)={0x2020}, 0x2020) */
/* write$FUSE_IOCTL(r1, &(0x7f0000000240)={0x20}, 0x20) */

#define _GNU_SOURCE

#include <fcntl.h>
#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/ioctl.h>
#include <sys/types.h>
#include <time.h>
#include <unistd.h>

#include <keyutils.h>

#include "hypercall.h"
#include "test.h"

#define O_NOTIFICATION_PIPE O_EXCL
#define KEYCTL_WATCH_KEY 0x20
#define IOC_WATCH_QUEUE_SET_SIZE 0x5760

key_serial_t key;
int r[2];

void *read_thread(void *arg) {
  char buf[1024];

  pin(2);

  hypercall(HCALL_INSTALL_BP, 0xffffffffffffffff, 1, 0);

  activate_bp_sync();

  syscall(SYS_SSB_SWITCH);
  if (read(r[0], buf, sizeof(buf)) == -1) {
    perror("read");
    exit(EXIT_FAILURE);
  }

  /* printf("%s", buf); */
  hypercall(HCALL_DEACTIVATE_BP, 0, 0, 0);

  return NULL;
}

void *ketctl_set_timeout_thread(void *arg) {
  pin(1);

  hypercall(HCALL_INSTALL_BP, 0xffffffff81c60edd, 0, 0);

  activate_bp_sync();

  syscall(SYS_SSB_SWITCH);
  keyctl(KEYCTL_SET_TIMEOUT, key, -1, 0);
  /* printf("ketctl_set_timeout_thread done\n"); */
  hypercall(HCALL_DEACTIVATE_BP, 0, 0, 0);
  return NULL;
}

void run() {
  hypercall(HCALL_RESET, 0, 0, 0);
  hypercall(HCALL_PREPARE, 2, 2, 0);

  hypercall(HCALL_ENABLE_KSSB, 0, 0, 0);

  key = add_key("cifs.spnego", "syz", 0, 0, KEY_SPEC_USER_KEYRING);
  if (key == -1) {
    perror("add_key");
    exit(EXIT_FAILURE);
  }

  /* printf("key: %x\n", key); */

  if (pipe2(r, O_NOTIFICATION_PIPE)) {
    perror("pipe2");
    exit(EXIT_FAILURE);
  }

  if (ioctl(r[0], IOC_WATCH_QUEUE_SET_SIZE, 0x10)) {
    perror("IOC_WATCH_QUEUE_SET_SIZE");
    exit(EXIT_FAILURE);
  }

  if (keyctl(KEYCTL_WATCH_KEY, key, r[1], 0)) {
    perror("keyctl(KEYCTL_WATCH_KEY)");
    exit(EXIT_FAILURE);
  }

  pthread_t pth1, pth2;
  pthread_create(&pth1, NULL, ketctl_set_timeout_thread, NULL);
  pthread_create(&pth2, NULL, read_thread, NULL);

  pthread_join(pth1, NULL);
  pthread_join(pth2, NULL);

  hypercall(HCALL_DISABLE_KSSB, 0, 0, 0);
  hypercall(HCALL_RESET, 0, 0, 0);
}

int main(int argc, char *argv[]) {
  pin(0);
  do_test();
  return 0;
}
