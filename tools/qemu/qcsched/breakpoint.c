#define _DEBUG

#include "qemu/osdep.h"

#include "exec/gdbstub.h"
#include "qemu/main-loop.h"
#include "sysemu/kvm.h"

#include "qemu/qcsched/qcsched.h"
#include "qemu/qcsched/vmi.h"
#include "qemu/qcsched/window.h"

#define RIP(cpu) (cpu->regs.rip)

// For the same reason for percpu_hw_breakpoint, I decide not to embed
// qcsched_trampoline_info in CPUState.
static struct qcsched_trampoline_info trampolines[MAX_NR_CPUS];

struct qcsched_trampoline_info *get_trampoline_info(CPUState *cpu)
{
    return &trampolines[cpu->cpu_index];
}

static void jump_into_trampoline(CPUState *cpu)
{
    RIP(cpu) = vmi_info.trampoline_entry_addr;
    cpu->qcsched_dirty = true;
}

static void __copy_registers(struct kvm_regs *dst, struct kvm_regs *src)
{
    *dst = *src;
}

// XXX: Disabling and restoring IRQ somehow blocks a thread going back
// from the trampoline. RelRazzer requires this to hold the store
// buffer but RazzerV2 does not. During RazzerV2 I do not use this,
// but when doing RelRazzer, we need to inspect what is
// happening. Repeatedly running qcsched-test-simple/bypass will be
// helpful.
__attribute__((unused)) static void __disable_irq(CPUState *cpu)
{
    cpu->qcsched_disable_irq = true;
}

__attribute__((unused)) static void __restore_irq(CPUState *cpu)
{
    cpu->qcsched_restore_irq = true;
}

static void kidnap_task(CPUState *cpu)
{
    struct qcsched_trampoline_info *trampoline = get_trampoline_info(cpu);

    if (sched.current == sched.total || !sched.activated)
        // We hit the last breakpoint. TODO: This if statement allows
        // thread execute parallel after the last breakpoint. We may
        // want to a better scheduling mechanism.
        return;

    // TODO: Do we want to kidnap more than one thread?
    ASSERT(!trampoline->trampoled, "kidnapping more than one thread");

    DRPRINTF(cpu, "kidnapping\n");
    __copy_registers(&trampoline->orig_regs, &cpu->regs);
    /* __disable_irq(cpu); */
    jump_into_trampoline(cpu);
    trampoline->trampoled = true;
    qcsched_arm_selfescape_timer(cpu);
}

static void resume_task(CPUState *cpu)
{
    struct qcsched_trampoline_info *trampoline = get_trampoline_info(cpu);

    ASSERT(trampoline->trampoled, "nothing has been kidnapped");
    // These two asserts should be enforced to safely run with
    // qcsched_handle_kick().
    ASSERT(qemu_mutex_iothread_locked(), "iothread mutex is not locked");
    ASSERT(cpu == current_cpu, "something wrong: cpu != current_cpu");

    DRPRINTF(cpu, "resumming (force: %d)\n", cpu->qcsched_force_wakeup);
    __copy_registers(&cpu->regs, &trampoline->orig_regs);
    /* __restore_irq(cpu); */
    cpu->qcsched_dirty = true;
    cpu->qcsched_force_wakeup = false;
    memset(trampoline, 0, sizeof(*trampoline) - sizeof(timer_t));

    qcsched_window_expand_window(cpu);
}

void wake_cpu_up(CPUState *cpu, CPUState *wakeup)
{
    int r;
    // Installing a breakpoint on the trampoline so each CPU can
    // wake up on its own.
    DRPRINTF(cpu, "waking cpu #%d\n", wakeup->cpu_index);
    r = kvm_insert_breakpoint_cpu(wakeup, vmi_info.trampoline_exit_addr, 1,
                                  GDB_BREAKPOINT_HW);
    // The race condition scenario: one cpu is trying to wake another
    // cpu up, and the one is also trying to wake up on its own. It is
    // okay in this case because we install the breakpoint anyway. So
    // ignore -EEXIST.
    ASSERT(r == 0 || r == -EEXIST, "failing to wake cpu #%d up err=%d",
           wakeup->cpu_index, r);
}

void wake_others_up(CPUState *cpu0)
{
    CPUState *cpu;
    struct qcsched_trampoline_info *trampoline;

    CPU_FOREACH(cpu)
    {
        trampoline = get_trampoline_info(cpu);
        if (!trampoline->trampoled || cpu->cpu_index == cpu0->cpu_index)
            continue;
        wake_cpu_up(cpu0, cpu);
    }
}

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
    DRPRINTF(cpu, "%s %llx\n", __func__, cpu->regs.rbx);
    // If the task can make a progress, we don't need to do something.
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

void qcsched_yield_turn(CPUState *cpu)
{
    // Hand over the baton to the next task
    hand_over_baton(cpu);
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
    DRPRINTF(cpu, "%s (%llx)\n", __func__, RIP(cpu));

    // This function handles a scheduling point regardless of that it
    // is behind of the current window focus.

    if (qcsched_window_hit_stale_schedpoint(cpu)) {
        // The general breakpoint handler already cleaned up left
        // schedpoint. Nothing to do with this breakpoint but just
        // expand the window.
        qcsched_window_expand_window(cpu);
        return;
    }

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
        qcsched_window_consecutive_schedpoint(cpu)) {
        // If the next scheduling point is not reachable because of
        // lock contention or installed on the same CPU, just keep
        // this CPU going
        qcsched_window_expand_window(cpu);
        qcsched_keep_this_cpu_going(cpu);
    } else {
        qcsched_yield_turn(cpu);
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
    ASSERT(++record->count < WATCHDOG_BREAKPOINT_COUNT_MAX,
           "watchdog failed: a breakpoint at %lx is hit %d times", record->RIP,
           record->count);
}

static void watchdog_breakpoint(CPUState *cpu)
{
    int index = cpu->cpu_index;
    struct qcsched_breakpoint_record *record = &sched.last_breakpoint[index];

    watchdog_breakpoint_check_count(cpu, record);

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
    ASSERT(!err || (err == -ENOENT && sched.activated == false),
           "failed to remove breakpoint err=%d\n", err);

    if (err)
        // XXX: I'm not sure this is a correct way to fix the
        // infinitely repeated breakpoint hit issue. Let's see what
        // will happen.
        kvm_update_guest_debug(cpu, 0);

    watchdog_breakpoint(cpu);

    // We need to synchronize thw window before cleaning up left
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
    ret = qcsched_handle_breakpoint_iolocked(cpu);
    qemu_mutex_unlock_iothread();
    return ret;
}

void qcsched_escape_if_trampoled(CPUState *cpu, CPUState *wakeup)
{
    struct qcsched_trampoline_info *trampoline = get_trampoline_info(wakeup);
    if (trampoline->trampoled)
        wake_cpu_up(cpu, wakeup);
}
