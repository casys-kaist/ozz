/* Test missing barrier bug in vlan.c
 * commit: c1102e9d49eb36c0be18cb3e16f6e46ffb717964
 * https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/commit/?id=c1102e9d49e
 */

#define _GNU_SOURCE

#include <errno.h>
#include <pthread.h>
#include <stdio.h>
#include <sys/socket.h>
#include <sys/ioctl.h>
#include <sys/uio.h>
#include <linux/if_vlan.h>
#include <linux/if_tun.h>
#include <linux/if.h>
#include <fcntl.h>

#include "test.h"

#define SIOCSIFVLAN	0x8983

#define DEV_NAME "tap0"
#define VLAN_NAME "tap0.1"
#define VLAN_VID 1

int sk, fd;
// todo: automate packet generation
/* Dummy ethernet frame with vlan tag */
char buf[] =
    "\xff\xff\xff\xff\xff\xff" /* Dest mac: any */
    "\xff\xff\xff\xff\xff\xff" /* Source mac: any */
    "\x81\x00\x00\x02"         /* Vlan tag: 8021q (2 byte), vlan id 02 (2 byte) */
    "\x08\x00"                 /* Length/protocol: ipv4 */
    "\x45\x00\x00\x2e\x00\x00\x00\x00\x40\x11\x88\x97\x05\x08\x07\x08" 
    "\x1e\x04\x10\x92\x10\x92\x00\x1a\x6d\xa3\x34\x33\x1f\x69\x40\x6b"
    "\x54\x59\xb6\x14\x2d\x11\x44\xbf\xaf\xd9\xbe\xaa"; /* Payload/crc: any? */

struct ifreq ifr = {
  .ifr_name = DEV_NAME,
  .ifr_flags = IFF_TAP|IFF_NO_PI,
};
struct ifreq ifr2 = {
  .ifr_name = DEV_NAME,
  .ifr_flags = IFF_UP,
};
struct vlan_ioctl_args if_request = {
  .cmd = ADD_VLAN_CMD,
  .device1 = DEV_NAME,
  .u.VID = VLAN_VID,
};
struct vlan_ioctl_args if_request2 = {
  .cmd = DEL_VLAN_CMD,
  .device1 = VLAN_NAME,
};

void *th1(void *ret) {
  pin(1);

  hypercall(HCALL_INSTALL_BP, 0xffffffff8dea2653, 0, 0); // Not working now

  activate_bp_sync();
  syscall(SYS_SSB_SWITCH);
  if (ioctl(sk, SIOCSIFVLAN, &if_request) < 0)
    perror("ioctl2");
  deactivate_bp_sync();
}

void *th2(void *ret) {
  pin(2);

  hypercall(HCALL_INSTALL_BP, 0xffffffffffffffff, 1, 0);

  activate_bp_sync();
  syscall(SYS_SSB_SWITCH);
  if (write(fd, buf, sizeof(buf)) < 0)
    perror("write");
  deactivate_bp_sync();
}

void run() {
  hypercall(HCALL_RESET, 0, 0, 0);
  hypercall(HCALL_PREPARE, 2, 2, 0);

  pthread_t pth1, pth2;
  
  pthread_create(&pth1, NULL, th1, NULL);
  pthread_create(&pth2, NULL, th2, NULL);

  pthread_join(pth1, NULL);
  pthread_join(pth2, NULL);

  if (ioctl(sk, SIOCSIFVLAN, &if_request2) < 0)
    perror("ioctl3");

  hypercall(HCALL_RESET, 0, 0, 0);
}

void tun_init() {
  sk = socket(AF_UNIX, SOCK_RAW, 0);
  fd = open("/dev/net/tun", O_RDWR);

  if (ioctl(fd, TUNSETIFF, &ifr) < 0) {
    perror("ioctl");
  }

  if (ioctl(sk, SIOCSIFFLAGS, &ifr2) < 0)
    perror("ioctl1");
}

void tun_finish() {
  close(sk);
  close(fd);
}

int main() {
  pin(0);
  tun_init();
  do_test(true);
  tun_finish();
  return 0;
}
