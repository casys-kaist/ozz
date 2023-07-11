#ifndef QCSCHED_EXECUITON_CONTROL_H
#define QCSCHED_EXECUITON_CONTROL_H

#include "qemu/osdep.h"

#include "cpu.h"

#ifdef CONFIG_QCSCHED_TRAMPOLINE
#include "qemu/qcsched/trampoline.h"
#else
#include "qemu/qcsched/blocker.h"
#endif

struct qcsched_exec_info {
#ifdef CONFIG_QCSCHED_TRAMPOLINE
    struct qcsched_trampoline_info trampoline;
#else
    struct qcsched_blocker_info blocker;
#endif
    bool kicked;
    timer_t timerid;
};

bool task_kidnapped(CPUState *cpu);
void kidnap_task(CPUState *cpu);
void resume_task(CPUState *cpu);
void wake_cpu_up(CPUState *cpu, CPUState *wakeup);
void wake_others_up(CPUState *cpu);
bool want_to_wake_up(CPUState *cpu);
void reset_exec_control(CPUState *cpu);

struct qcsched_exec_info *get_exec_info(CPUState *cpu);

#endif
