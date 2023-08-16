#define _DEBUG

#include "qemu/osdep.h"

#include <linux/kvm.h>

#include "cpu.h"
#include "qemu/main-loop.h"
#include "sysemu/kvm.h"

#include "qemu/qcsched/exec_control.h"
#include "qemu/qcsched/qcsched.h"
#include "qemu/qcsched/vmi.h"

#include <sys/syscall.h>
#include <sys/types.h>
#include <time.h>
#include <unistd.h>

bool warn_once[warn_once_total];

struct qcsched sched;

bool qcsched_pre_run(CPUState *cpu)
{
    g_assert(!qemu_mutex_iothread_locked());
    if (cpu->qcsched_dirty) {
        ASSERT(!kvm_write_registers(cpu, &cpu->regs),
               "failed to write registers");
        cpu->qcsched_dirty = false;
    }
#ifdef CONFIG_QCSCHED_TRAMPOLINE
    return true;
#else
    return !task_kidnapped(cpu);
#endif
}

void qcsched_post_run(CPUState *cpu)
{
    ASSERT(!kvm_read_registers(cpu, &cpu->regs), "failed to read registers");
#ifndef CONFIG_QCSCHED_TRAMPOLINE
    qemu_mutex_lock_iothread();
    if (want_to_wake_up(cpu)) {
        DRPRINTF(cpu, "I want to wake up\n");
        resume_task(cpu);
    }
    qemu_mutex_unlock_iothread();
#endif
}

// NOTE: The man page for sigevent clearly specifies that struct
// sigevent has a member field 'sigev_notify_thread_id'. Indeed, the
// struct does not have the member field and, instead, it is defined
// as the macro below (see include/uapi/asm-generic/siginfo.h in the
// Linux repo). For some reasons, the macro in the header file does
// not work, so I copied it here as a workaround.
#define sigev_notify_thread_id _sigev_un._tid

#define gettid() syscall(SYS_gettid)

void qcsched_init_vcpu(CPUState *cpu)
{
    struct qcsched_exec_info *info = get_exec_info(cpu);
    struct sigevent sevp;
    pid_t tid = gettid();
    sevp.sigev_notify = SIGEV_THREAD_ID;
    sevp.sigev_signo = SIG_IPI;
    sevp.sigev_value.sival_int = TRAMPOLINE_ESCAPE_MAGIC;
    sevp.sigev_notify_thread_id = tid;
    ASSERT(!timer_create(CLOCK_MONOTONIC, &sevp, &info->timerid),
           "timer_create");
}
