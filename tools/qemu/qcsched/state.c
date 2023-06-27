#include <stdbool.h>

#include "qemu/osdep.h"

#include "cpu.h"
#include "hw/core/cpu.h"
#include "qemu/main-loop.h"

#include "qemu/qcsched/constant.h"
#include "qemu/qcsched/qcsched.h"
#include "qemu/qcsched/state.h"

bool qcsched_cpu_transition(CPUState *cpu, enum qcsched_cpu_state from,
                            enum qcsched_cpu_state to)
{
    g_assert(qemu_mutex_iothread_locked());
    enum qcsched_cpu_state *cur = &sched.cpu_state[cpu->cpu_index];
    if (*cur != from)
        return false;
    *cur = to;
    return true;
}

static bool __cpu_is_used(CPUState *cpu)
{
    return 1 <= cpu->cpu_index && cpu->cpu_index < 1 + sched.nr_cpus;
}

bool qcsched_check_all_cpu_state(enum qcsched_cpu_state state)
{
    CPUState *cpu;
    g_assert(qemu_mutex_iothread_locked());
    CPU_FOREACH(cpu)
    {
        if (!__cpu_is_used(cpu))
            continue;

        if (!qcsched_check_cpu_state(cpu, state))
            return false;
    }
    return true;
}

bool qcsched_check_cpu_state(CPUState *cpu, enum qcsched_cpu_state state)
{
    return sched.cpu_state[cpu->cpu_index] >= state;
}

void qcsched_set_cpu_state(CPUState *cpu, enum qcsched_cpu_state state)
{
    sched.cpu_state[cpu->cpu_index] = state;
}
