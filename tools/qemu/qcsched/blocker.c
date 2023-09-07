#define _DEBUG

#include "qemu/osdep.h"

#include "cpu.h"

#include "qemu/qcsched/exec_control.h"

static struct qcsched_blocker_info *get_blocker_info(CPUState *cpu)
{
    struct qcsched_exec_info *info = get_exec_info(cpu);
    return &info->blocker;
}

bool blocker_task_kidnapped(CPUState *cpu)
{
    struct qcsched_blocker_info *blocker = get_blocker_info(cpu);
    return blocker->blocked;
}

void blocker_kidnap_task(CPUState *cpu)
{
    struct qcsched_blocker_info *blocker = get_blocker_info(cpu);
    blocker->blocked = true;
    blocker->waking_up = false;
}

void blocker_resume_task(CPUState *cpu)
{
    struct qcsched_blocker_info *blocker = get_blocker_info(cpu);
    blocker->blocked = false;
    blocker->waking_up = false;
    cpu->qcsched_force_wakeup = false;
    // XXX: As with trampoline_resume_task(), I reset info->kicked
    // without knowing that this is correct.
    struct qcsched_exec_info *info = (struct qcsched_exec_info *)blocker;
    info->kicked = false;
}

void blocker_wake_cpu_up(CPUState *cpu, CPUState *wakeup)
{
    struct qcsched_blocker_info *blocker = get_blocker_info(wakeup);
    blocker->waking_up = true;
}

bool blocker_want_to_wake_up(CPUState *cpu)
{
    struct qcsched_blocker_info *blocker = get_blocker_info(cpu);
    return blocker->waking_up;
}

void blocker_reset(CPUState *cpu)
{
    struct qcsched_blocker_info *blocker = get_blocker_info(cpu);
    blocker->waking_up = false;
    blocker->blocked = false;
}
