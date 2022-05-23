#include <stdio.h>
#include <sys/types.h>
#include <sys/socket.h>
#include <net/if.h>
#include <linux/rfkill.h>
#include <pthread.h>
#include <sys/epoll.h>
#include <sys/ioctl.h>
#include <sys/socket.h>
#include <sys/uio.h>
#include <fcntl.h>
#include <unistd.h>
#include <errno.h>
#include <string.h>
#include <stdarg.h>

const int kFailStatus = 67;

void doexit(int status)
{
	_exit(status);
	for (;;) {
	}
}

void failmsg(const char* err, const char* msg, ...)
{
	int e = errno;
	fprintf(stderr, "SYZFAIL: %s\n", err);
	if (msg) {
		va_list args;
		va_start(args, msg);
		vfprintf(stderr, msg, args);
		va_end(args);
	}
	fprintf(stderr, " (errno %d: %s)\n", e, strerror(e));
	doexit(kFailStatus);
}

void fail(const char* err)
{
	failmsg(err, 0);
}

typedef unsigned long long uint64;
typedef unsigned int uint32;
typedef unsigned short uint16;
typedef unsigned char uint8;

#define BTPROTO_HCI 1

struct vhci_vendor_pkt {
	uint8 type;
	uint8 opcode;
	uint16 id;
};

#define HCI_VENDOR_PKT 0xff

static int vhci_fd = -1;

static void initialize_vhci() {
	vhci_fd = open("/dev/vhci", O_RDWR);
	if (vhci_fd == -1)
		fail("open /dev/vhci failed");

	printf("vhci_fd=%d\n", vhci_fd);

	// Remap vhci onto higher fd number to hide it from fuzzer and to keep
	// fd numbers stable regardless of whether vhci is opened or not (also see kMaxFd).
	const int kVhciFd = 202;
	if (dup2(vhci_fd, kVhciFd) < 0)
		fail("dup2(vhci_fd, kVhciFd) failed");
	close(vhci_fd);
	vhci_fd = kVhciFd;
	printf("now vhci_fd=%d\n", vhci_fd);

	sleep(1);

	for (int i = 0; i < 10; i++) {
	  struct vhci_vendor_pkt vendor_pkt;
	  if (read(vhci_fd, &vendor_pkt, sizeof(vendor_pkt)) != sizeof(vendor_pkt))
		fail("read failed");

	  printf("read done %d\n", i);

	  if (vendor_pkt.type == HCI_VENDOR_PKT)
		break;
	  /* if (vendor_pkt.type != HCI_VENDOR_PKT) */
	  /* 	failmsg("wrong response packet", "expected=%d, got=%d", HCI_VENDOR_PKT, vendor_pkt.type); */
	}

	printf("closing vhci_fd\n");
	close(vhci_fd);
}

int main() {
  initialize_vhci();
  printf("Okay\n");
  return 0;
}
