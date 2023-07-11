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
}

void blocker_resume_task(CPUState *cpu)
{
    struct qcsched_blocker_info *blocker = get_blocker_info(cpu);
    blocker->blocked = false;
}

void blocker_wake_cpu_up(CPUState *cpu, CPUState *wakeup)
{
    resume_task(wakeup);
}
