#ifndef __QCSCHED_STATE_H
#define __QCSCHED_STATE_H

#include "qemu/osdep.h"

#include "cpu.h"

#include "qemu/qcsched/constant.h"

enum qcsched_cpu_state {
    qcsched_cpu_idle = 0,
    qcsched_cpu_ready,
    qcsched_cpu_activated,
    qcsched_cpu_deactivated
};

bool qcsched_cpu_transition(CPUState *cpu, enum qcsched_cpu_state from,
                            enum qcsched_cpu_state to);

bool qcsched_check_all_cpu_state(enum qcsched_cpu_state state);
bool qcsched_check_cpu_state(CPUState *cpu, enum qcsched_cpu_state state);
void qcsched_set_cpu_state(CPUState *cpu, enum qcsched_cpu_state state);

#endif /* __QCSCHED_STATE_H */
