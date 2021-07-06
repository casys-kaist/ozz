#define _DEBUG

#include "qemu/osdep.h"

#include "cpu.h"

#include "qemu/qcsched/qcsched.h"
#include "qemu/qcsched/vmi.h"

struct qcsched_vmi_info vmi_info;

void qcsched_vmi_set_trampoline(CPUState *cpu, target_ulong addr)
{
    DRPRINTF(cpu, "Trampoline addr: %lx\n", addr);
    vmi_info.trampoline_addr = addr;
}

void qcsched_vmi_set_hook(CPUState *cpu, target_ulong addr)
{
    DRPRINTF(cpu, "Hook addr: %lx\n", addr);
    vmi_info.hook_addr = addr;
}

void qcsched_vmi_set__per_cpu_offset(CPUState *cpu, int index, target_ulong addr)
{
    DRPRINTF(cpu, "__per_cpu_offset[%d]: %lx\n", index, addr);
    vmi_info.__per_cpu_offset[index] = addr;
}

void qcsched_vmi_set_current_task(CPUState *cpu, target_ulong addr)
{
    DRPRINTF(cpu, "current_task: %lx\n", addr);
    vmi_info.current_task = addr;
}

void qcsched_vmi_task(CPUState *cpu, struct qcsched_vmi_task *t)
{
    // In x86_64, every task has a its own stack, and each CPU has
    // additional one stack for serving IRQs.
    // Let's use the frame pointer as a task id until we have a better
    // option.
    t->stack = cpu->regs.rbp;
}

static bool __vmi_same_task(struct qcsched_vmi_task *t0,
                            struct qcsched_vmi_task *t1)
{
    return t0->stack == t1->stack;
}

bool qcsched_vmi_can_progress(CPUState *cpu)
{
    struct qcsched_vmi_task running;
    qcsched_vmi_task(cpu, &running);
    return __vmi_same_task(&running, &(sched.current->t));
}
