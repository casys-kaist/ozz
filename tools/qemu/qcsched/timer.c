#define _DEBUG

#include "qemu/osdep.h"

#include "exec/gdbstub.h"
#include "qemu/main-loop.h"
#include "sysemu/kvm.h"

#include "qemu/qcsched/qcsched.h"
#include "qemu/qcsched/vmi.h"

#include <signal.h>
#include <sys/syscall.h>
#include <sys/types.h>
#include <time.h>
#include <unistd.h>

static void __init_itimerspec(struct itimerspec *its)
{
    memset(&its->it_interval, 0, sizeof(its->it_interval));
    its->it_value = (struct timespec){.tv_sec = 1, .tv_nsec = 100};
}

void qcsched_arm_selfescape_timer(CPUState *cpu)
{
    struct itimerspec its;
    struct qcsched_trampoline_info *trampoline = get_trampoline_info(cpu);

    __init_itimerspec(&its);

    ASSERT(!timer_settime(trampoline->timerid, 0, &its, NULL), "timer_settime");
}
