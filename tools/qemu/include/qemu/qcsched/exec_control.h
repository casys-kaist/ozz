#ifndef QCSCHED_EXECUITON_CONTROL_H
#define QCSCHED_EXECUITON_CONTROL_H

#include "qemu/osdep.h"

#include "cpu.h"

#ifdef CONFIG_QCSCHED_TRAMPOLINE
#include "qemu/qcsched/trampoline.h"
#else
#include "qemu/qcsched/suspend.h"
#endif

struct qcsched_exec_info {
#ifdef CONFIG_QCSCHED_TRAMPOLINE
    struct qcsched_trampoline_info trampoline;
#else
    struct qcsched_suspend_info info;
#endif
    bool kicked;
    // timerid should be the last member because of resume_task().
    timer_t timerid;
};

bool task_kidnapped(CPUState *cpu);
void kidnap_task(CPUState *cpu);
void resume_task(CPUState *cpu);
void wake_cpu_up(CPUState *cpu, CPUState *wakeup);
void wake_others_up(CPUState *cpu);

void qcsched_escape_if_kidnapped(CPUState *cpu, CPUState *wakeup);

struct qcsched_exec_info *get_exec_info(CPUState *cpu);

#endif
