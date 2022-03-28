#define _DEBUG

#include "qemu/osdep.h"

#include <linux/kvm.h>

#include "cpu.h"
#include "exec/gdbstub.h"
#include "qemu-common.h"
#include "qemu/main-loop.h"
#include "sysemu/cpus.h"
#include "sysemu/kvm.h"

#include "qemu/qcsched/hcall.h"
#include "qemu/qcsched/qcsched.h"
#include "qemu/qcsched/state.h"
#include "qemu/qcsched/vmi.h"

static bool qcsched_entry_used(struct qcsched_entry *entry)
{
    return !!entry->schedpoint.addr;
}

static bool sanitize_breakpoint(struct qcsched *sched)
{
    int i;

    if (!sched->total)
        return false;

    for (i = 0; i < sched->total; i++) {
        if (!qcsched_entry_used(&sched->entries[i]))
            return false;
    }
    return true;
}

static void __remove_breakpoints_and_escape_cpu(CPUState *this,
                                                CPUState *remote)
{
    ASSERT(!sched.activated,
           "trying to remove breakpoints while the schedule is activated");
    // Do not remove all breakpoints since some may be installed on
    // the trampoline.
    for (int i = 0; i < sched.total; i++) {
        struct qcsched_entry *entry = &sched.entries[i];
        if (entry->cpu == remote->cpu_index)
            kvm_remove_breakpoint_cpu(remote, entry->schedpoint.addr, 1,
                                      GDB_BREAKPOINT_HW);
    }
    qcsched_escape_if_trampoled(this, remote);
}

static void qcsched_reset_window(CPUState *cpu)
{
    struct qcsched_schedpoint_window *window =
        &sched.schedpoint_window[cpu->cpu_index];

    window->total = 0;
    window->activated = 0;
    window->from = window->until = END_OF_SCHEDPOINT_WINDOW;
    window->left_behind = END_OF_SCHEDPOINT_WINDOW;
    window->cpu = cpu->cpu_index;
}

static void qcsched_reset(CPUState *cpu)
{
    CPUState *cpu0;
    DRPRINTF(cpu, "%s\n", __func__);

    // This hcall hard reset a previous schedule. If a executor thread
    // abnormally exited, a garbage schedule still resides in the
    // hypervisor. Fuzzer needs to reset it before executing the next
    // schedule.

    sched.used = true;
    sched.activated = false;

    // NOTE: qcsched_reset() should be executed in CPU0, and all other
    // worker CPUs should be executed in CPU with the index other than
    // 0. Otherwise, qcsched_reset() and other hcalls can race causing
    // a deadlock.
    CPU_FOREACH(cpu0)
    {
        if (cpu0->cpu_index == 0)
            continue;

        if (!qcsched_check_cpu_state(cpu0, qcsched_cpu_deactivated))
            __remove_breakpoints_and_escape_cpu(cpu, cpu0);

        memset(&sched.last_breakpoint[cpu0->cpu_index], 0,
               sizeof(struct qcsched_breakpoint_record));

        qcsched_set_cpu_state(cpu0, qcsched_cpu_idle);
        qcsched_reset_window(cpu0);
        qcsched_vmi_lock_info_reset(cpu0);
    }
    sched.total = sched.current = 0;
    sched.nr_cpus = 0;
    memset(&sched.entries, 0, sizeof(struct qcsched_entry) * MAX_SCHEDPOINTS);
}

static target_ulong qcsched_prepare(CPUState *cpu, unsigned int nr_bps,
                                    unsigned int nr_cpus)
{
    DRPRINTF(cpu, "%s\n", __func__);
    DRPRINTF(cpu, "nr_bps: %u\n", nr_bps);

    if (sched.total != 0)
        return -EBUSY;

    if (!vmi_info.hook_addr)
        return -EINVAL;

    if (nr_bps >= MAX_SCHEDPOINTS)
        return -EINVAL;

    if (nr_cpus >= MAX_CPUS)
        return -EINVAL;

    sched.total = nr_bps;
    sched.nr_cpus = nr_cpus;
    sched.used = false;

    return 0;
}

static target_ulong
qcsched_install_breakpoint(CPUState *cpu, target_ulong addr, int order,
                           enum qcschedpoint_footprint footprint)
{
    struct qcsched_entry *entry;
    struct qcsched_schedpoint_window *window;

#ifdef _DEBUG_VERBOSE
    DRPRINTF(cpu, "%s\n", __func__);
    DRPRINTF(cpu, "addr: %lx, order: %d, footprint: %d\n", addr, order,
             footprint);
#endif

    if (!sched.total)
        return -EINVAL;

    if (sched.total <= order)
        return -EINVAL;

    // Allowed: idle
    if (qcsched_check_cpu_state(cpu, qcsched_cpu_ready))
        return -EINVAL;

    entry = &sched.entries[order];

    if (qcsched_entry_used(entry))
        return -EBUSY;

    if (footprint != 1)
        // We will skip entries with footprint 1. All others are
        // benign.
        footprint = 0;

    entry->schedpoint = (struct qcschedpoint){
        .addr = addr, .order = order, .footprint = footprint};
    entry->cpu = cpu->cpu_index;
    qcsched_vmi_task(cpu, &entry->t);

    window = &sched.schedpoint_window[cpu->cpu_index];
    if (window->from == END_OF_SCHEDPOINT_WINDOW || window->from > order)
        // until and from are same before activating, which means the
        // window is [from, from), which means the window is empty.
        window->until = window->from = order;

    return 0;
}

static void do_activate_breakpoint(CPUState *cpu)
{
    struct qcsched_entry *entry;
    struct qcsched_schedpoint_window *window;
    int err, i;
    bool need_hook = false;

    // We don't install scheduling points on the master CPU
    if (cpu->cpu_index == 0)
        return;

    window = &sched.schedpoint_window[cpu->cpu_index];

    // We don't install breakpoints until the hook is hit. Instead we
    // count the number of scheduling points that will be installed on
    // this CPU.
    for (i = 0; i < sched.total; i++) {
        entry = &sched.entries[i];
        if (entry->cpu != cpu->cpu_index)
            continue;
        window->total++;
        need_hook = true;
    }

    if (!need_hook)
        return;

    ASSERT(!(err = kvm_insert_breakpoint_cpu(cpu, vmi_info.hook_addr, 1,
                                             GDB_BREAKPOINT_HW)),
           "failed to insert a breakpoint at the hook err=%d\n", err);
}

static target_ulong qcsched_activate_breakpoint(CPUState *cpu)
{
    DRPRINTF(cpu, "%s\n", __func__);

    if (sched.used)
        return -EINVAL;

    // Allowed: idle, ready
    if (qcsched_check_cpu_state(cpu, qcsched_cpu_activated))
        return -EINVAL;

    qcsched_set_cpu_state(cpu, qcsched_cpu_ready);

    if (!qcsched_check_all_cpu_state(qcsched_cpu_ready))
        return -EAGAIN;

    if (!sanitize_breakpoint(&sched))
        return -EINVAL;

    if (!qcsched_cpu_transition(cpu, qcsched_cpu_ready, qcsched_cpu_activated))
        return -EINVAL;

    // At this point, we assume all CPUs are ready and all schedules
    // are sanitized.
    do_activate_breakpoint(cpu);

    if (!sched.activated) {
        sched.activated = true;
        sched.current = 0;
    }

    return 0;
}

static target_ulong qcsched_deactivate_breakpoint(CPUState *cpu)
{
    CPUState *cpu0;

    DRPRINTF(cpu, "%s\n", __func__);

    // Allowed: ready, activated, deactivated
    if (!qcsched_check_cpu_state(cpu, qcsched_cpu_ready))
        return -EINVAL;

    qcsched_set_cpu_state(cpu, qcsched_cpu_deactivated);

    qcsched_window_close_window(cpu);

    if (sched.activated) {
        // NOTE: two reasons for falsifying sched.activated here: 1)
        // to prevent a race condition during removing bps on other
        // CPUs, and 2) let the trampoled CPUs see sched.activated as
        // false so it can resume (see. qcsched_vmi_can_progress()
        // called in __handle_breakpoint_hook()).
        sched.activated = false;

        // We don't want to reuse the schedule.
        sched.used = true;

        CPU_FOREACH(cpu0)
        {
            if (cpu0->cpu_index == 0)
                continue;
            __remove_breakpoints_and_escape_cpu(cpu, cpu0);
        }
    }

    return 0;
}

static target_ulong qcsched_footprint_breakpoint(CPUState *cpu,
                                                 target_ulong cnt_uptr,
                                                 target_ulong data_uptr,
                                                 target_ulong retry_uptr)
{
    struct qcsched_entry *entry;
    target_ulong footprintul, cntul, retryul;
    int i, idx;

    DRPRINTF(cpu, "%s\n", __func__);

    if (!qcsched_check_cpu_state(cpu, qcsched_cpu_deactivated))
        return -EINVAL;

    cntul = 0;
    retryul = 0;
    for (i = 0, idx = 0; i < sched.total; i++) {
        entry = &sched.entries[i];
        if (entry->cpu != cpu->cpu_index)
            continue;

        if (entry->schedpoint.footprint == footprint_not_addressed ||
            entry->schedpoint.footprint == footprint_dropped)
            retryul = 1;

        footprintul = (target_ulong)entry->schedpoint.footprint;
#ifdef _DEBUG_VERBOSE
        DRPRINTF(cpu, "footprint at %d: %lu\n", i, footprintul);
#endif
        ASSERT(!cpu_memory_rw_debug(cpu, data_uptr + idx, &footprintul,
                                    sizeof(target_ulong), 1),
               "Can't write order");
        idx += 8;
        cntul++;
    }

#ifdef _DEBUG_VERBOSE
    DRPRINTF(cpu, "local entries %lu\n", cntul);
#endif
    ASSERT(!cpu_memory_rw_debug(cpu, cnt_uptr, &cntul, sizeof(target_ulong), 1),
           "Can't write count");
    DRPRINTF(cpu, "Retry: %lu\n", retryul);
    ASSERT(!cpu_memory_rw_debug(cpu, retry_uptr, &retryul, sizeof(target_ulong),
                                1),
           "Can't write retry");
    return 0;
}

static target_ulong qcsched_clear_breakpoint(CPUState *cpu)
{
    struct qcsched_entry *entry;
    int i;

    DRPRINTF(cpu, "%s\n", __func__);

    if (sched.activated)
        return -EBUSY;

    if (!qcsched_cpu_transition(cpu, qcsched_cpu_deactivated, qcsched_cpu_idle))
        return -EINVAL;

    for (i = 0; i < sched.total; i++) {
        entry = &sched.entries[i];
        if (entry->cpu != cpu->cpu_index)
            continue;
        memset(entry, 0, sizeof(struct qcsched_entry));
    }
    // Calling this hcall means the syscall has been finished. We can
    // remove all breakpoints on this CPU
    kvm_remove_all_breakpoints_cpu(cpu);
    return 0;
}

void qcsched_handle_hcall(CPUState *cpu, struct kvm_run *run)
{
    __u64 *args = run->hypercall.args;
    __u64 cmd = args[0];
    target_ulong ret = 0;
    int order;
    unsigned int nr_bps, nr_cpus;
    target_ulong addr, subcmd, misc;
    target_ulong data, retry;
    enum qcschedpoint_footprint footprint;

    qemu_mutex_lock_iothread();
    switch (cmd) {
    case HCALL_RESET:
        qcsched_reset(cpu);
        break;
    case HCALL_PREPARE:
        nr_bps = args[1];
        nr_cpus = args[2];
        ret = qcsched_prepare(cpu, nr_bps, nr_cpus);
        break;
    case HCALL_INSTALL_BP:
        addr = args[1];
        order = args[2];
        footprint = args[3];
        ret = qcsched_install_breakpoint(cpu, addr, order, footprint);
        break;
    case HCALL_ACTIVATE_BP:
        ret = qcsched_activate_breakpoint(cpu);
        break;
    case HCALL_DEACTIVATE_BP:
        ret = qcsched_deactivate_breakpoint(cpu);
        break;
    case HCALL_FOOTPRINT_BP:
        addr = args[1];
        data = args[2];
        retry = args[3];
        ret = qcsched_footprint_breakpoint(cpu, addr, data, retry);
        break;
    case HCALL_CLEAR_BP:
        ret = qcsched_clear_breakpoint(cpu);
        break;
    case HCALL_VMI_HINT:
        subcmd = args[1];
        addr = args[2];
        misc = args[3];
        ret = qcsched_vmi_hint(cpu, subcmd, addr, misc);
        break;
    case HCALL_ENABLE_KSSB:
        ret = qcsched_enable_kssb(cpu);
        break;
    case HCALL_DISABLE_KSSB:
        ret = qcsched_disable_kssb(cpu);
        break;
    default:
        ret = -EINVAL;
        break;
    }

#ifdef _DEBUG_VERBOSE
    DRPRINTF(cpu, "ret: %lx\n", ret);
#else
    if (ret != 0) {
        if (cmd == HCALL_INSTALL_BP)
            DRPRINTF(cpu, "HCALL_INSTALL_BP\n");
        DRPRINTF(cpu, "ret: %lx\n", ret);
    }
#endif
    qemu_mutex_unlock_iothread();

    if (!qcsched_jumped_into_trampoline(cpu))
        qcsched_commit_state(cpu, ret);
}
