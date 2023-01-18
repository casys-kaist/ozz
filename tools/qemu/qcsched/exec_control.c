#define _DEBUG

#include "qemu/osdep.h"

#include "exec/gdbstub.h"
#include "qemu/main-loop.h"
#include "sysemu/kvm.h"

#include "qemu/qcsched/exec_control.h"
#include "qemu/qcsched/qcsched.h"
#include "qemu/qcsched/vmi.h"
#include "qemu/qcsched/window.h"

// For the same reason for percpu_hw_breakpoint, I decide not to embed
// qcsched_exec_info in CPUState.
static struct qcsched_exec_info infos[MAX_NR_CPUS];

struct qcsched_exec_info *get_exec_info(CPUState *cpu)
{
    return &infos[cpu->cpu_index];
}

bool task_kidnapped(CPUState *cpu)
{
#ifdef CONFIG_QCSCHED_TRAMPOLINE
    return trampoline_task_kidnapped(cpu);
#else
    return false;
#endif
}

void kidnap_task(CPUState *cpu)
{
    bool kidnapped = task_kidnapped(cpu);

    ASSERT(qcsched_vmi_running_context_being_scheduled(cpu),
           "kidnapping a wrong context");

    if (sched.current == sched.total || !sched.activated)
        // We hit the last breakpoint. TODO: This if statement allows
        // thread execute parallel after the last breakpoint. We may
        // want to a better scheduling mechanism.
        return;

    ASSERT(!kidnapped, "kidnapping more than one thread, cpu=%d",
           cpu->cpu_index);

    DRPRINTF(cpu, "kidnapping\n");
#ifdef CONFIG_QCSCHED_TRAMPOLINE
    trampoline_kidnap_task(cpu);
#else
#endif
    qcsched_arm_selfescape_timer(cpu);
}

void resume_task(CPUState *cpu)
{
    bool kidnapped = task_kidnapped(cpu);

    ASSERT(kidnapped, "nothing has been kidnapped");
    // These two asserts should be enforced to safely run with
    // qcsched_handle_kick().
    ASSERT(qemu_mutex_iothread_locked(), "iothread mutex is not locked");
    ASSERT(cpu == current_cpu, "something wrong: cpu != current_cpu");

    DRPRINTF(cpu, "resumming (force: %d)\n", cpu->qcsched_force_wakeup);
#ifdef CONFIG_QCSCHED_TRAMPOLINE
    trampoline_resume_task(cpu);
#else
#endif
    qcsched_window_expand_window(cpu);
}

void wake_cpu_up(CPUState *cpu, CPUState *wakeup)
{
    int r;
    // Installing a breakpoint on the trampoline so each CPU can
    // wake up on its own.
    DRPRINTF(cpu, "waking cpu #%d\n", wakeup->cpu_index);
    r = kvm_insert_breakpoint_cpu(wakeup, vmi_info.trampoline_exit_addr, 1,
                                  GDB_BREAKPOINT_HW);
    // The race condition scenario: one cpu is trying to wake another
    // cpu up, and the one is also trying to wake up on its own. It is
    // okay in this case because we install the breakpoint anyway. So
    // ignore -EEXIST.
    ASSERT(r == 0 || r == -EEXIST, "failing to wake cpu #%d up err=%d",
           wakeup->cpu_index, r);
}

void wake_others_up(CPUState *cpu0)
{
    CPUState *cpu;
    bool kidnapped;

    CPU_FOREACH(cpu)
    {
        kidnapped = task_kidnapped(cpu);
        if (!kidnapped || cpu->cpu_index == cpu0->cpu_index)
            continue;
        wake_cpu_up(cpu0, cpu);
    }
}
