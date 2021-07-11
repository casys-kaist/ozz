#define _DEBUG

#include "qemu/osdep.h"

#include <linux/kvm.h>

#include "cpu.h"
#include "exec/gdbstub.h"
#include "qemu-common.h"
#include "qemu/main-loop.h"
#include "sysemu/kvm.h"

#include "qemu/qcsched/hcall.h"
#include "qemu/qcsched/qcsched.h"
#include "qemu/qcsched/vmi.h"

static bool qcsched_entry_used(struct qcsched_entry *entry)
{
    return !!entry->schedpoint.addr;
}

static bool sanitize_breakpoint(struct qcsched *sched)
{
    int i;
    for (i = 0; i < sched->total; i++) {
        if (!qcsched_entry_used(&sched->entries[i]))
            return false;
    }
    return true;
}

static target_ulong qcsched_install_breakpoint(CPUState *cpu, target_ulong addr,
                                               int order)
{
    struct qcsched_entry *entry = &sched.entries[order];

    DRPRINTF(cpu, "%s\n", __func__);
    DRPRINTF(cpu, "addr: %lx, order: %d\n", addr, order);

    if (qcsched_entry_used(entry))
        return -EBUSY;

    entry->schedpoint = (struct qcschedpoint){.addr = addr, .order = order};
    entry->cpu = cpu->cpu_index;
    qcsched_vmi_task(cpu, &entry->t);
    sched.total++;
    return 0;
}

static target_ulong qcsched_activate_breakpoint(CPUState *cpu0)
{
    int total, i;
    bool need_hook;
    CPUState *cpu;
    struct qcsched_entry *entry;

    DRPRINTF(cpu0, "%s\n", __func__);

    if (sched.activated)
        return -EBUSY;

    if (!vmi_info.hook_addr)
        return -EINVAL;

    if (!sanitize_breakpoint(&sched))
        return -EINVAL;

    total = sched.total;

    // NOTE: kvm_insert_breakpoint_cpu() releases qemu_global_mutex
    // during run_on_cpu() and another CPU may acquire the mutex,
    // resulting in more than one CPU being in this function. To
    // prevent breakpoints from being installed multiple times, set
    // sched.activated true before installing breakpoints so the
    // latter CPU returns early.
    sched.activated = true;
    sched.current = 0;

    CPU_FOREACH(cpu)
    {
        need_hook = false;
        for (i = 0; i < total; i++) {
            entry = &sched.entries[i];
            if (entry->cpu == cpu->cpu_index) {
                DRPRINTF(cpu, "Installing a breakpoint at %lx on cpu#%d\n",
                         entry->schedpoint.addr, entry->cpu);
                ASSERT(!kvm_insert_breakpoint_cpu(cpu, entry->schedpoint.addr,
                                                  1, GDB_BREAKPOINT_HW),
                       "failed to insert a breakpiont at a scheduling point\n");
                need_hook = true;
            }
        }
        if (!need_hook)
            continue;
        ASSERT(!kvm_insert_breakpoint_cpu(cpu, vmi_info.hook_addr, 1,
                                          GDB_BREAKPOINT_HW),
               "failed to insert a breakpoint at the hook\n");
    }
    return 0;
}

static target_ulong qcsched_deactivate_breakpoint(CPUState *cpu0)
{
    int total, i;
    CPUState *cpu;
    struct qcsched_entry *entry;

    DRPRINTF(cpu0, "%s\n", __func__);

    if (!sched.activated)
        return -EINVAL;

    total = sched.total;

    CPU_FOREACH(cpu)
    {
        for (i = 0; i < total; i++) {
            entry = &sched.entries[i];
            if (entry->cpu == cpu->cpu_index)
                kvm_remove_breakpoint_cpu(cpu, entry->schedpoint.addr, 1,
                                          GDB_BREAKPOINT_HW);
        }
    }
    sched.activated = false;
    return 0;
}

static target_ulong qcsched_clear_breakpoint(CPUState *cpu0)
{
    CPUState *cpu;

    DRPRINTF(cpu0, "%s\n", __func__);

    CPU_FOREACH(cpu) { kvm_remove_all_breakpoints_cpu(cpu); }
    memset(&sched, 0, sizeof(struct qcsched));
    return 0;
}

void qcsched_handle_hcall(CPUState *cpu, struct kvm_run *run)
{
    __u64 *args = run->hypercall.args;
    __u64 cmd = args[0];
    int order;
    target_ulong addr, subcmd;
    target_ulong hcall_ret;

    qemu_mutex_lock_iothread();
    switch (cmd) {
    case HCALL_INSTALL_BP:
        addr = args[1];
        order = args[2];
        hcall_ret = qcsched_install_breakpoint(cpu, addr, order);
        break;
    case HCALL_ACTIVATE_BP:
        hcall_ret = qcsched_activate_breakpoint(cpu);
        break;
    case HCALL_DEACTIVATE_BP:
        hcall_ret = 0;
        hcall_ret = qcsched_deactivate_breakpoint(cpu);
        break;
    case HCALL_CLEAR_BP:
        hcall_ret = qcsched_clear_breakpoint(cpu);
        break;
    case HCALL_VMI_HINT:
        subcmd = args[1];
        addr = args[2];
        hcall_ret = qcsched_vmi_hint(cpu, subcmd, addr);
        break;
    default:
        hcall_ret = -EINVAL;
        break;
    }
    DRPRINTF(cpu, "ret: %lx\n", hcall_ret);
    qemu_mutex_unlock_iothread();

    qcsched_commit_state(cpu, hcall_ret);
}
