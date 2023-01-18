#define _DEBUG

#include "qemu/osdep.h"

#include "exec/gdbstub.h"
#include "qemu/main-loop.h"
#include "sysemu/kvm.h"

#include "qemu/qcsched/exec_control.h"
#include "qemu/qcsched/qcsched.h"
#include "qemu/qcsched/vmi.h"
#include "qemu/qcsched/window.h"

static bool breakpoint_on_hook(CPUState *cpu)
{
    return RIP(cpu) == vmi_info.hook_addr;
}

static bool breakpoint_on_trampoline(CPUState *cpu)
{
    return RIP(cpu) == vmi_info.trampoline_entry_addr ||
           RIP(cpu) == vmi_info.trampoline_exit_addr;
}

static bool breakpoint_on_schedpoint(CPUState *cpu)
{
    struct qcsched_entry *entry;
    struct qcsched_vmi_task running;
    int i;

    qcsched_vmi_task(cpu, &running);

    for (i = 0; i < sched.total; i++) {
        entry = &sched.entries[i];
        if (entry->schedpoint.addr == RIP(cpu) && entry->breakpoint.installed &&
            vmi_same_task(&running, &entry->t))
            return true;
    }
    return false;
}

static void __handle_breakpoint_hook(CPUState *cpu)
{
    int err;

    DRPRINTF(cpu, "%s %llx\n", __func__, cpu->regs.rbx);

    if (!qcsched_vmi_running_context_being_scheduled(cpu)) {
        // The context is switched, this is not a thread we want to
        // control. Reinstall the brekapoint on the hook.
        ASSERT(!(err = kvm_insert_breakpoint_cpu(cpu, vmi_info.hook_addr, 1,
                                                 GDB_BREAKPOINT_HW)),
               "failed to insert a breakpoint at the hook err=%d\n", err);
        return;
    }

    if (!qcsched_vmi_can_progress(cpu))
        kidnap_task(cpu);
    else
        qcsched_window_expand_window(cpu);
}

static void __handle_breakpoint_trampoline(CPUState *cpu)
{
    DRPRINTF(cpu, "%s\n", __func__);
    // Each cpu determines that it can make a progress.
    if (qcsched_vmi_can_progress(cpu))
        resume_task(cpu);
}

void qcsched_yield_turn_from(CPUState *cpu, int current_order)
{
    // Hand over the baton to the next task
    hand_over_baton_from(cpu, current_order);
    // and then kidnap the executing task
    kidnap_task(cpu);
    // And then wake others up
    wake_others_up(cpu);
}

void qcsched_keep_this_cpu_going(CPUState *cpu)
{
    int step;
    struct qcsched_schedpoint_window *window =
        &sched.schedpoint_window[cpu->cpu_index];
    struct qcsched_entry *this_cpu_next =
        lookup_entry_by_order(cpu, window->from);

    if (!this_cpu_next)
        // We are done. Move the focus to the end of the schedule
        step = sched.total - sched.current;
    else
        step = this_cpu_next->schedpoint.order - sched.current;

    forward_focus(cpu, step);
}

static void __handle_breakpoint_schedpoint(CPUState *cpu)
{
    struct qcsched_entry *current_entry;
    int current_order;

    DRPRINTF(cpu, "%s (%llx)\n", __func__, RIP(cpu));

    // XXX: still we are facing double-kidnapping. Is this
    // workardound find...?
    if (task_kidnapped(cpu))
        return;

    // This function handles a scheduling point regardless of that it
    // is behind of the current window focus.

    if (qcsched_window_hit_stale_schedpoint(cpu)) {
        // The general breakpoint handler already cleaned up left
        // schedpoint. Nothing to do with this breakpoint but just
        // expand the window.
        qcsched_window_expand_window(cpu);
        return;
    }

    current_entry = lookup_entry_by_address(cpu, cpu->regs.rip);
    current_order = current_entry->schedpoint.order;

    // Prune out missed schedpoints first
    qcsched_window_prune_missed_schedpoint(cpu);
    // Leave the footprint before we shrink the window
    qcsched_window_leave_footprint(cpu, footprint_hit);
    // Shrink the schedpoint window
    qcsched_window_shrink_window(cpu);

    // NOTE: At this point window->from points to the next scheduling
    // point in its scheduling window, and sched.current points to the
    // current focus (i.e., not moved forward yet). Below function
    // calls should be aware of this.
    if (qcsched_window_lock_contending(cpu) ||
        qcsched_window_consecutive_schedpoint(cpu, current_order)) {
        // If the next scheduling point is not reachable because of
        // lock contention or installed on the same CPU, just keep
        // this CPU going
        qcsched_window_expand_window(cpu);
        qcsched_keep_this_cpu_going(cpu);
    } else {
        qcsched_yield_turn_from(cpu, current_order);
    }
}

static void
watchdog_breakpoint_check_count(CPUState *cpu,
                                struct qcsched_breakpoint_record *record)
{
    if (record->RIP != RIP(cpu))
        return;
    // In this project, there is no case that a breakpoint keep being
    // hit consecutively so far (we don't consider cases where an
    // instruction is executed multiple times, such as a loop; this
    // will be addressed in the future). So if a breakpoint is hit
    // multiple times in a row, something goes wrong (e.g., race
    // condition in QEMU). This watchdog detects it early.
    record->count++;
    ASSERT(record->count < WATCHDOG_BREAKPOINT_COUNT_KILL_QEMU,
           "watchdog failed: killing QEMU");

    if (record->count >= WATCHDOG_BREAKPOINT_COUNT_MAX) {
        int err;
        DRPRINTF(cpu, "watchdog failed: a breakpoint at %lx is hit %d times",
                 record->RIP, record->count);
        qcsched_window_close_window(cpu);
        ASSERT(!(err = kvm_update_guest_debug(cpu, 0)),
               "%s, kvm_update_guest_debug_debug returns %d", __func__, err);
    }
}

static void watchdog_breakpoint(CPUState *cpu)
{
    int index = cpu->cpu_index;
    struct qcsched_breakpoint_record *record = &sched.last_breakpoint[index];

    if (breakpoint_on_hook(cpu))
        // XXX: Because a breakpoint on the hook can be hit by
        // multiple threads, it is not a subject of the watchdog. But
        // we many want to kill the schedule if too many threads hit
        // it to prevent the performance degradation.
        return;

    watchdog_breakpoint_check_count(cpu, record);

    if (record->RIP == RIP(cpu))
        return;

    record->RIP = RIP(cpu);
    record->count = 0;
}

static int qcsched_handle_breakpoint_iolocked(CPUState *cpu)
{
    // Remove the breakpoint first
    int err = kvm_remove_breakpoint_cpu(cpu, RIP(cpu), 1, GDB_BREAKPOINT_HW);
    // When removing a breakpoint on another CPU,
    // kvm_remove_breakpoint_cpu() temporary releases the iolock. This
    // opens a chance of race condition between this function and a
    // function removing a breakpoint on this CPU, and consequently,
    // kvm_remove_breakpoint_cpu() can return -ENOENT. Since the only
    // location that removes breakpoints on other CPUs is
    // qcsched_deacitavte_breakpoint() which falsify sched.activated,
    // we can check sched.activated to confirm that the error code is
    // actually benign.
    if (err && !(err == -ENOENT && sched.activated == false)) {
        // Let's abort the schedule
        DRPRINTF(cpu,
                 "Failed to remove a breakpoint (error=%d). Abort a schedule\n",
                 err);
        qcsched_window_close_window(cpu);
        ASSERT(!(err = kvm_update_guest_debug(cpu, 0)),
               "%s, kvm_update_guest_debug_debug returns %d", __func__, err);
        return 0;
    }

    if (!qcsched_vmi_in_task(cpu))
        // XXX: Temporary workaround of schedpoint that can be hit by
        // {,soft}IRQ contexts. By removing the breakpoint and then
        // doing nothing, we can deal with such breakpoints as missing
        // ones. This is obviously not a better way of dealing with
        // them, so we may want to fix this.
        return 0;

    watchdog_breakpoint(cpu);

    // We need to synchronize the window before cleaning up left
    // schedpoint
    qcsched_window_sync(cpu);
    qcsched_window_cleanup_left_schedpoint(cpu);

    if (breakpoint_on_hook(cpu)) {
        __handle_breakpoint_hook(cpu);
    } else if (breakpoint_on_trampoline(cpu)) {
        __handle_breakpoint_trampoline(cpu);
    } else if (breakpoint_on_schedpoint(cpu)) {
        __handle_breakpoint_schedpoint(cpu);
    } else {
        // Two cases: 1) unknown breakpoint, which may be an error, 2)
        // cleaned up before.
        DRPRINTF(cpu, "Ignore breakpoint: %llx\n", RIP(cpu));
    }
    return 0;
}

int qcsched_handle_breakpoint(CPUState *cpu)
{
    int ret;
    qemu_mutex_lock_iothread();
    qcsched_eat_cookie(cpu, cookie_breakpoint);
    ret = qcsched_handle_breakpoint_iolocked(cpu);
    qemu_mutex_unlock_iothread();
    return ret;
}

void qcsched_escape_if_kidnapped(CPUState *cpu, CPUState *wakeup)
{
    bool kidnapped = task_kidnapped(wakeup);
    if (kidnapped)
        wake_cpu_up(cpu, wakeup);
}
