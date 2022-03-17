#define _DEBUG

#include "qemu/osdep.h"

#include "cpu.h"

#include "qemu/qcsched/hcall_constant.h"
#include "qemu/qcsched/qcsched.h"
#include "qemu/qcsched/vmi.h"

struct qcsched_vmi_info vmi_info;

static void qcsched_vmi_hint_trampoline(CPUState *cpu, target_ulong addr,
                                        int index)
{
    DRPRINTF(cpu, "trampoline %s addr : %lx\n", (!index ? "entry" : "exit"),
             addr);
    vmi_info.trampoline_addr[index] = addr;
}

static void qcsched_vmi_hint_hook(CPUState *cpu, target_ulong addr)
{
    DRPRINTF(cpu, "hook addr: %lx\n", addr);
    vmi_info.hook_addr = addr;
}

static void qcsched_vmi_hint__per_cpu_offset(CPUState *cpu, int index,
                                             target_ulong addr)
{
    DRPRINTF(cpu, "__per_cpu_offset[%d]: %lx\n", index, addr);
    vmi_info.__per_cpu_offset[index] = addr;
}

static void qcsched_vmi_hint_current_task(CPUState *cpu, target_ulong addr)
{
    DRPRINTF(cpu, "current_task: %lx\n", addr);
    vmi_info.current_task = addr;
}

static void qcsched_vmi_hint__ssb_do_emulate(CPUState *cpu, target_ulong addr)
{
    DRPRINTF(cpu, "__ssb_do_dmulate: %lx\n", addr);
    vmi_info.__ssb_do_emulate = addr;
}

static void qcsched_vmi_critical_section_enter(CPUState *cpu,
                                               target_ulong lockdep_addr)
{
    struct qcsched_vmi_lock_info *lock_info =
        &vmi_info.lock_info[cpu->cpu_index];
    int cnt = lock_info->count;

    // Can't hold more lock info
    if (cnt >= MAX_LOCKS)
        return;

    lock_info->acquired[cnt] = lockdep_addr;
    lock_info->count = cnt + 1;
}

static void qcsched_vmi_critical_section_exit(CPUState *cpu,
                                              target_ulong lockdep_addr)
{
    struct qcsched_vmi_lock_info *lock_info =
        &vmi_info.lock_info[cpu->cpu_index];
    int cnt = lock_info->count;

    for (int i = 0; i < cnt; i++) {
        if (lockdep_addr == lock_info->acquired[i]) {
            lock_info->acquired[i] = lock_info->acquired[cnt - 1];
            lock_info->count--;
            return;
        }
    }
}

target_ulong qcsched_vmi_hint(CPUState *cpu, target_ulong type,
                              target_ulong addr)
{
    int index;
    switch (type) {
    case VMI_TRAMPOLINE ... VMI_TRAMPOLINE + 1:
        index = type - VMI_TRAMPOLINE;
        qcsched_vmi_hint_trampoline(cpu, addr, index);
        break;
    case VMI_HOOK:
        qcsched_vmi_hint_hook(cpu, addr);
        break;
    case VMI__PER_CPU_OFFSET0 ... VMI__PER_CPU_OFFSET0 + 63:
        index = type - VMI__PER_CPU_OFFSET0;
        qcsched_vmi_hint__per_cpu_offset(cpu, index, addr);
        break;
    case VMI_CURRENT_TASK:
        qcsched_vmi_hint_current_task(cpu, addr);
        break;
    case VMI__SSB_DO_EMULATE:
        qcsched_vmi_hint__ssb_do_emulate(cpu, addr);
        break;
    case VMI_CRITICAL_SECTION_ENTER:
        qcsched_vmi_critical_section_enter(cpu, addr);
        break;
    case VMI_CRITICAL_SECTION_EXIT:
        qcsched_vmi_critical_section_exit(cpu, addr);
        break;
    default:
        DRPRINTF(cpu, "Unknown VMI type: %lx\n", type);
        return -EINVAL;
    }
    return 0;
}

void qcsched_vmi_lock_info_reset(CPUState *cpu)
{
    struct qcsched_vmi_lock_info *lock_info =
        &vmi_info.lock_info[cpu->cpu_index];
    lock_info->count = 0;
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
           vmi_same_task(&running, &entry->t) || sched.total == sched.current ||
           cpu->qcsched_force_wakeup || !sched.activated;
}
