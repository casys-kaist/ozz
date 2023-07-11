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
    return blocker_task_kidnapped(cpu);
#endif
}

void kidnap_task(CPUState *cpu)
{
    bool kidnapped = task_kidnapped(cpu);

    ASSERT(qcsched_vmi_running_context_being_scheduled(cpu, true),
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
    blocker_kidnap_task(cpu);
#endif
    qcsched_arm_selfescape_timer(cpu);
}

void resume_task(CPUState *cpu)
{
    bool kidnapped = task_kidnapped(cpu);

    ASSERT(kidnapped, "nothing has been kidnapped %d", cpu->cpu_index);
    // These two asserts should be enforced to safely run with
    // qcsched_handle_kick().
    ASSERT(qemu_mutex_iothread_locked(), "iothread mutex is not locked");
    ASSERT(cpu == current_cpu, "something wrong: cpu != current_cpu");

    DRPRINTF(cpu, "resumming (force: %d)\n", cpu->qcsched_force_wakeup);
#ifdef CONFIG_QCSCHED_TRAMPOLINE
    trampoline_resume_task(cpu);
#else
    blocker_resume_task(cpu);
#endif
    qcsched_window_expand_window(cpu);
}

void wake_cpu_up(CPUState *cpu, CPUState *wakeup)
{
    DRPRINTF(cpu, "waking cpu #%d\n", wakeup->cpu_index);
#ifdef CONFIG_QCSCHED_TRAMPOLINE
    trampolione_wake_cpu_up(cpu, wakeup);
#else
    blocker_wake_cpu_up(cpu, wakeup);
#endif
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

bool want_to_wake_up(CPUState *cpu)
{
#ifdef CONFIG_QCSCHED_TRAMPOLINE
    return false;
#else
    return blocker_want_to_wake_up(cpu);
#endif
}

void reset_exec_control(CPUState *cpu)
{
#ifndef CONFIG_QCSCHED_TRAMPOLINE
    blocker_reset(cpu);
#endif
}
