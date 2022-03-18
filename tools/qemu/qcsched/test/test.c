#define _GNU_SOURCE

#include <err.h>
#include <fcntl.h>
#include <linux/kvm.h>
#include <pthread.h>
#include <sched.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <sys/ioctl.h>
#include <sys/mman.h>
#include <sys/syscall.h>
#include <unistd.h>

// TODO:

int predicted_fd = -1;
int vm;

#include "hypercall.h"

#define gettid() syscall(SYS_gettid)

#ifdef SPINLOCK_TEST
__thread int fd;
__thread struct kmemcov_access *cover;
#endif

enum kmemcov_access_type {
    KMEMCOV_ACCESS_STORE,
    KMEMCOV_ACCESS_LOAD,
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

static void set_affinity(int cpu)
{
    cpu_set_t set;
    CPU_ZERO(&set);
    CPU_SET(cpu, &set);
    if (sched_setaffinity(gettid(), sizeof(set), &set))
        perror("sched_setaffinity");
}

struct schedpoint {
    unsigned long long addr;
    int order;
};

static void install_schedpoint(struct schedpoint *sched, int size)
{
    printf("hihi %d\n", size);
    for (int i = 0; i < size; i++) {
        hypercall(HCALL_INSTALL_BP, sched[i].addr, sched[i].order, 0);
    }
    unsigned long ret;
#define EAGAIN 11
    int cnt = 10;
    do {
        ret = hypercall(HCALL_ACTIVATE_BP, 0, 0, 0);
        usleep(5 * 1000);
    } while (ret == -EAGAIN && --cnt);
}

static void th_init(void)
{
#ifdef SPINLOCK_TEST
    fd = open("/sys/kernel/debug/kmemcov", O_RDWR);
    if (fd == -1)
        perror("open"), exit(1);
    /* Setup trace mode and trace size. */
    if (ioctl(fd, KMEMCOV_INIT_TRACE, COVER_SIZE))
        perror("ioctl"), exit(1);
    /* Mmap buffer shared between kernel- and user-space. */
    cover = (struct kmemcov_access *)mmap(
        NULL, COVER_SIZE * sizeof(struct kmemcov_access),
        PROT_READ | PROT_WRITE, MAP_SHARED, fd, 0);
    if ((void *)cover == MAP_FAILED)
        perror("mmap"), exit(1);
    /* Enable coverage collection on the current thread. */
    if (ioctl(fd, KMEMCOV_ENABLE, 0))
        perror("ioctl"), exit(1);
#endif
}

static void th_clear(void)
{
#ifdef SPINLOCK_TEST
    if (ioctl(fd, KMEMCOV_DISABLE, 0))
        perror("ioctl"), exit(1);
    /* Free resources. */
    if (munmap(cover, COVER_SIZE * sizeof(struct kmemcov_access)))
        perror("munmap"), exit(1);
    if (close(fd))
        perror("close"), exit(1);
#endif
}

static void clear_schedpoint(void)
{
    hypercall(HCALL_DEACTIVATE_BP, 0, 0, 0);
    hypercall(HCALL_CLEAR_BP, 0, 0, 0);
}

static void *th1(void *dummy)
{
    set_affinity(1);
    th_init();
    struct schedpoint sched[] = {
#ifdef CVE20196974
#include "schedpoint/cve-2019-6974-1.h"
#endif
#ifdef CVE20196974_MINIMAL
#include "schedpoint/cve-2019-6974-minimal-1.h"
#endif
#if defined(SIMPLE_TEST) || defined(SPINLOCK_TEST)
#include "schedpoint/simple-1.h"
#endif
#ifdef BYPASS_TEST
#include "schedpoint/bypass-1.h"
#endif
    };
    install_schedpoint(sched, sizeof(sched) / sizeof(sched[0]));
#if defined(CVE20196974) || defined(CVE20196974_MINIMAL)
    struct kvm_create_device cd = {.type = KVM_DEV_TYPE_VFIO,
                                   .fd = -1, // outparm
                                   .flags = 0};
    ioctl(vm, KVM_CREATE_DEVICE, &cd);
#endif
#if defined(SIMPLE_TEST) || defined(BYPASS_TEST) || defined(SPINLOCK_TEST)
    int typ = 1;
#ifdef SPINLOCK_TEST
    typ = 2;
#endif
#define SYS_qcshed_simple_write 509
    syscall(SYS_qcshed_simple_write, typ);
#endif
    clear_schedpoint();
    th_clear();
    return NULL;
}

static void *th2(void *dummy)
{
    set_affinity(2);
    th_init();
    struct schedpoint sched[] = {
#ifdef CVE20196974
#include "schedpoint/cve-2019-6974-2.h"
#endif
#ifdef CVE20196974_MINIMAL
#include "schedpoint/cve-2019-6974-minimal-2.h"
#endif
#if defined(SIMPLE_TEST) || defined(SPINLOCK_TEST)
#include "schedpoint/simple-2.h"
#endif
#ifdef BYPASS_TEST
#include "schedpoint/bypass-2.h"
#endif
    };

    install_schedpoint(sched, sizeof(sched) / sizeof(sched[0]));
#if defined(CVE20196974) || defined(CVE20196974_MINIMAL)
    close(predicted_fd);
#endif
#if defined(SIMPLE_TEST) || defined(BYPASS_TEST) || defined(SPINLOCK_TEST)
    int typ = 1;
#ifdef SPINLOCK_TEST
    typ = 2;
#endif
#define SYS_qcshed_simple_read 510
    syscall(SYS_qcshed_simple_read, typ);
#endif
    clear_schedpoint();
    th_clear();
    return NULL;
}

static void init()
{
#if defined(CVE20196974) || defined(CVE20196974_MINIMAL)
    predicted_fd = -1;
    int kvm = open("/dev/kvm", O_RDWR);
    if (kvm == -1)
        perror("open");
    vm = ioctl(kvm, KVM_CREATE_VM, 0);
    if (vm == -1)
        perror("KVM_CREATE_VM");
    predicted_fd = dup(0);
    close(predicted_fd);
#endif
}

int main(void)
{
    pthread_t pth1, pth2;
    int nr_bps = -1;

    set_affinity(0);
#ifdef CVE20196974
    nr_bps = 22;
#endif
#ifdef CVE20196974_MINIMAL
    nr_bps = 2;
#endif
#if defined(SIMPLE_TEST) || defined(BYPASS_TEST) || defined(SPINLOCK_TEST)
    nr_bps = 20;
#endif
    hypercall(HCALL_RESET, 0, 0, 0);
    hypercall(HCALL_PREPARE_BP, nr_bps, 2, 0);
    hypercall(HCALL_ENABLE_KSSB, 0, 0, 0);

    init();

    pthread_create(&pth1, NULL, th1, NULL);
    pthread_create(&pth2, NULL, th2, NULL);
    pthread_join(pth1, NULL);
    pthread_join(pth2, NULL);

    hypercall(HCALL_DISABLE_KSSB, 0, 0, 0);
}
