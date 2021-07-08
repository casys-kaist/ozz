#define _DEBUG

#include "qemu/osdep.h"

#include "cpu.h"

#include "qemu/qcsched/qcsched.h"
#include "qemu/qcsched/vmi.h"

struct qcsched_vmi_info vmi_info;

void qcsched_vmi_set_trampoline(CPUState *cpu, target_ulong addr, int index)
{
    DRPRINTF(cpu, "trampoline %s addr : %lx\n", (!index ? "entry" : "exit"),
             addr);
    vmi_info.trampoline_addr[index] = addr;
}

void qcsched_vmi_set_hook(CPUState *cpu, target_ulong addr)
{
    DRPRINTF(cpu, "hook addr: %lx\n", addr);
    vmi_info.hook_addr = addr;
}

void qcsched_vmi_set__per_cpu_offset(CPUState *cpu, int index,
                                     target_ulong addr)
{
    DRPRINTF(cpu, "__per_cpu_offset[%d]: %lx\n", index, addr);
    vmi_info.__per_cpu_offset[index] = addr;
}

void qcsched_vmi_set_current_task(CPUState *cpu, target_ulong addr)
{
    DRPRINTF(cpu, "current_task: %lx\n", addr);
    vmi_info.current_task = addr;
}

static target_ulong current_task(CPUState *cpu)
{
    // TODO: This only works for x86_64
    uint8_t buf[128];
    target_ulong task, pcpu_ptr,
        __per_cpu_offset = vmi_info.__per_cpu_offset[cpu->cpu_index];

    if (__per_cpu_offset == 0)
        return 0;

    pcpu_ptr = __per_cpu_offset + vmi_info.current_task;

    ASSERT(!cpu_memory_rw_debug(cpu, pcpu_ptr, buf, TARGET_LONG_SIZE, 0),
           "Can't read pcpu section");

    task = *(target_ulong *)buf;
    return task;
}

void qcsched_vmi_task(CPUState *cpu, struct qcsched_vmi_task *t)
{
    // Use the current pointer in x86_64 until we have a better
    // option. It is stored in the per-cpu pointer called
    // current_task.
    t->task_struct = current_task(cpu);
}

bool vmi_same_task(struct qcsched_vmi_task *t0, struct qcsched_vmi_task *t1)
{
    return t0->task_struct == t1->task_struct;
}

static bool __vmi_scheduling_subject(struct qcsched_vmi_task *t)
{
    // We don't have that many entries. Just iterating is fast enough.
    int i;
    for (i = 0; i < sched.total; i++) {
        if (vmi_same_task(t, &sched.entries[i].t))
            return true;
    }
    return false;
}

bool qcsched_vmi_can_progress(CPUState *cpu)
{
    struct qcsched_entry *entry = &sched.entries[sched.current];
    struct qcsched_vmi_task running;
    qcsched_vmi_task(cpu, &running);
    return !__vmi_scheduling_subject(&running) ||
           vmi_same_task(&running, &entry->t) || sched.total == sched.current;
}
