#ifndef QCSCHED_BLOCKER_H
#define QCSCHED_BLOCKER_H

#include "qemu/osdep.h"

#include "cpu.h"

struct qcsched_blocker_info {
    bool blocked;
};

bool blocker_task_kidnapped(CPUState *cpu);
void blocker_kidnap_task(CPUState *cpu);
void blocker_resume_task(CPUState *cpu);
void blocker_wake_cpu_up(CPUState *cpu, CPUState *wakeup);

#endif
