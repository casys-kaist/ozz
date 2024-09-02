/* Test missing read memory barrier in fs/file.c
 * Related patch: 7ee47dcfff1835ff ("fs: use acquire ordering in __fget_light()")
 */

#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif

#include <fcntl.h>
#include <poll.h>
#include <pthread.h>
#include <signal.h>
#include <stdio.h>
#include <sys/ioctl.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <unistd.h>
#include <sys/socket.h>

#include "hypercall.h"
#include "test.h"

#define __NR_close_range 436
#define CLOSE_RANGE_UNSHARE	(1U << 1)

#define ADDR_SCHED0 0xffffffff8238943d
#define ADDR_SCHED1 0xffffffff82387099
#define ADDR_SCHED2 0xffffffff8238125d

#define ADDR_REORDER 0xffffffff823870ca

#define FD_EXTEND 256

#define DIRNAME "/root"

static inline int sys_close_range(unsigned int fd, unsigned int max_fd,
				  unsigned int flags)
{
	return syscall(__NR_close_range, fd, max_fd, flags);
}

int fd;

void *th1(void *a) {
    pin(1);
    hypercall(HCALL_INSTALL_BP, ADDR_SCHED0, 0, 0);
    hypercall(HCALL_INSTALL_BP, ADDR_SCHED2, 2, 0);
    activate_bp_sync();
    syscall(SYS_SSB_SWITCH);

    int res = dup2(fd, FD_EXTEND);
    close(FD_EXTEND);
    sys_close_range(fd, fd, CLOSE_RANGE_UNSHARE);

    if (res < 0)
        perror("dup2() failed");
    printf("dup2: %d\n", res);

    hypercall(HCALL_DEACTIVATE_BP, 0, 0, 0);
    return NULL;
}

void *th2(void *a) {
    pin(2);
    hypercall(HCALL_INSTALL_BP, ADDR_SCHED1, 1, 0);
    hypercall(HCALL_INSTALL_BP, 0xffffffffffffffff, 3, 0);
    activate_bp_sync();
    syscall(SYS_SSB_SWITCH);

    int res = fchdir(FD_EXTEND);
    if (res < 0)
        perror("fchdir() failed");
    printf("fchdir: %d\n", res);

    hypercall(HCALL_DEACTIVATE_BP, 0, 0, 0);
    return NULL;
}

void run() {
    hypercall(HCALL_PREPARE, 4, 2, 0);

    fd = open(DIRNAME, O_RDONLY);
    if (fd < 0) {
        perror("open() failed");
        return;
    }
    printf("open: %d\n", fd);

    pthread_t pth1;
    pthread_create(&pth1, NULL, th1, NULL);
    // If we create two threads, the count become at least 2, avoiding bug location.
    // so execute here instead of new thread.
    th2(NULL);
    pthread_join(pth1, NULL);

    close(fd);
    return;
}

struct kssb_flush_table_entry {
    unsigned long inst;
    int value;
    void *pad1, *pad2;
};

int main() {
    pin(0);
    hypercall(HCALL_RESET, 0, 0, 0);
    hypercall(HCALL_ENABLE_KSSB, 0, 0, 0);
    int vec[1] = {1};
    struct kssb_flush_table_entry table[] = {
        {ADDR_REORDER, 0},
    };
    syscall(SYS_SSB_FEEDINPUT, &vec, 1, &table, 1);
    run();
    hypercall(HCALL_DISABLE_KSSB, 0, 0, 0);
    return 0;
}