#define _GNU_SOURCE
#include <stdio.h>
#include <stddef.h>
#include <stdint.h>
#include <stdlib.h>
#include <sys/types.h>
#include <sys/stat.h>
#include <sys/ioctl.h>
#include <sys/mman.h>
#include <unistd.h>
#include <fcntl.h>
#include <sys/syscall.h>
#include <pthread.h>
#include <stdbool.h>

#include <linux/aio_abi.h>

#include "hypercall.h"

#define KCOV_INIT_TRACE			_IOR('c', 1, unsigned long)
#define KCOV_ENABLE			_IO('c', 100)
#define KCOV_DISABLE			_IO('c', 101)
#define COVER_SIZE			(64<<10)

#define KCOV_TRACE_PC  0
#define KCOV_TRACE_CMP 1

#define EINVAL 22

#define SYS_SSB_FEEDINPUT 500
#define SYS_SBITMAP_INIT 505
#define SYS_SBITMAP_FUNC1 506
#define SYS_SBITMAP_FUNC2 507
#define SYS_SBITMAP_CLEAR 508

#define io_setup(...) syscall(SYS_io_setup, ## __VA_ARGS__)
#define io_submit(...) syscall(SYS_io_submit, ## __VA_ARGS__)
#define io_destroy(...) syscall(SYS_io_destroy, ## __VA_ARGS__)

#define gettid() syscall(SYS_gettid)

char buf[8192] __attribute__((aligned(8192)));
int fd2;
aio_context_t ctx = 0;
bool go = false;

unsigned long breakpoint_addr[2];
unsigned long get_breakpoint_addr(void) {
	char buf[128];
	FILE *fp = popen("grep 'kssb_test_breakpoint' /proc/kallsyms | head -n 1 | cut -d' ' -f1", "r");
	fgets(buf, sizeof(buf), fp);
	pclose(fp);
	return strtoul(buf, NULL, 16);
}

unsigned long get_breakpoint_addr2(void) {
	char buf[128];
	FILE *fp = popen("grep 'kssb_test_breakpoint2' /proc/kallsyms | head -n 1 | cut -d' ' -f1", "r");
	fgets(buf, sizeof(buf), fp);
	pclose(fp);
	return strtoul(buf, NULL, 16);
}

void *thr(void *_arg)
{
	int cpu = (int)(intptr_t)_arg;
	int ret;
	cpu_set_t set;

	if (cpu == 3)
		usleep(100 * 1000);

	CPU_SET(cpu, &set);
	if (sched_setaffinity(gettid(), sizeof(set), &set))
		perror("sched_setaffinity");
#ifdef _KCOV
	int fd;
	unsigned long *cover, n, i;
	/* A single fd descriptor allows coverage collection on a single
	 * thread.
	 */
	fd = open("/sys/kernel/debug/kcov", O_RDWR);
	if (fd == -1)
		perror("open"), exit(1);
	/* Setup trace mode and trace size. */
	if (ioctl(fd, KCOV_INIT_TRACE, COVER_SIZE))
		perror("ioctl"), exit(1);
	/* Mmap buffer shared between kernel- and user-space. */
	cover = (unsigned long*)mmap(NULL, COVER_SIZE * sizeof(unsigned long),
								 PROT_READ | PROT_WRITE, MAP_SHARED, fd, 0);
	if ((void*)cover == MAP_FAILED)
		perror("mmap"), exit(1);
	/* Enable coverage collection on the current thread. */
	if (ioctl(fd, KCOV_ENABLE, KCOV_TRACE_PC))
		perror("ioctl"), exit(1);
	/* Reset coverage from the tail of the ioctl() call. */
	__atomic_store_n(&cover[0], 0, __ATOMIC_RELAXED);
#endif
	struct iocb iocb = {
		.aio_data = 0,
		.aio_key = 0,
		.aio_rw_flags = RWF_DSYNC,
		.aio_lio_opcode = IOCB_CMD_PREAD,
		.aio_reqprio = 0,
		.aio_fildes = fd2,
		.aio_buf = (long long unsigned int)buf,
		.aio_nbytes = 4096,
		.aio_offset = 0,
	};
	struct iocb *iocbp[1] = { &iocb };

	int i = cpu - 2;
	hypercall(HCALL_INSTALL_BP, breakpoint_addr[i], i, 0);

	while(!go);

	ret = io_submit(ctx, 1, &iocbp);
	if (ret == -1)
		perror("io_submit"), exit(1);
	hypercall(HCALL_DEACTIVATE_BP, 0, 0, 0);
	hypercall(HCALL_CLEAR_BP, 0, 0, 0);
#ifdef _KCOV
	Read number of PCs collected.
	n = __atomic_load_n(&cover[0], __ATOMIC_RELAXED);
	for (i = 0; i < n; i++)
		printf("0x%lx\n", cover[i + 1]);
	/* Disable coverage collection for the current thread. After this call
	 * coverage can be enabled for a different thread.
	 */
	if (ioctl(fd, KCOV_DISABLE, 0))
		perror("ioctl"), exit(1);
	/* Free resources. */
	if (munmap(cover, COVER_SIZE * sizeof(unsigned long)))
		perror("munmap"), exit(1);
	if (close(fd))
		perror("close"), exit(1);
#endif
	return NULL;
}

int main(int argc, char **argv)
{
	int ret, cnt = 5;
	int flush_vector[] = {0, 1};
	cpu_set_t set;
	unsigned long hcall_ret;

	breakpoint_addr[0] = get_breakpoint_addr();
	breakpoint_addr[1] = get_breakpoint_addr2();

	CPU_ZERO(&set);
	CPU_SET(0, &set);
	if (sched_setaffinity(gettid(), sizeof(set), &set))
		perror("sched_setaffinity");

	hypercall(HCALL_ENABLE_KSSB, 0, 0, 0);

	hypercall(HCALL_PREPARE_BP, 2, 0, 0);

	syscall(SYS_SSB_FEEDINPUT, flush_vector, 2);

	fd2 = open("/dev/nullb0", O_RDONLY | O_DIRECT);
	if (fd2 == -1)
		perror("open /dev/nullb0"), exit(1);
	ret = io_setup(10, &ctx);
	if (ret == -1)
		perror("io_setup"), exit(1);

	/* That's the target syscal call. */
	pthread_t pth1, pth2;
	pthread_create(&pth1, NULL, thr, (void*)(intptr_t)2);
	pthread_create(&pth2, NULL, thr, (void*)(intptr_t)3);

	do {
		hcall_ret = hypercall(HCALL_ACTIVATE_BP, 0, 0, 0);
		usleep(100 * 1000);
	} while(hcall_ret == -EINVAL && --cnt);

	go = true;

	pthread_join(pth1, NULL);
	pthread_join(pth2, NULL);

	ret = io_destroy(ctx);
	if (ret == -1)
		perror("io_destroy"), exit(1);
	
	return 0;
}
