#define _DEBUG

#include "qemu/osdep.h"

#include "cpu.h"
#include "qemu/main-loop.h"
#include "sysemu/kvm.h"

#include "qemu/qcsched/qcsched.h"
#include "qemu/qcsched/window.h"

#define schedpoint_window_full(window)                                         \
    (window->activated == SCHEDPOINT_WINDOW_SIZE)
#define schedpoint_window_empty(window) (window->activated == 0)

struct qcsched_entry *lookup_entry_by_order(CPUState *cpu, int from)
{
    if (from == END_OF_SCHEDPOINT_WINDOW)
        return NULL;
    for (int i = from; i < sched.total; i++) {
        struct qcsched_entry *entry = &sched.entries[i];
        if (cpu && entry->cpu != cpu->cpu_index)
            continue;
        if (entry->schedpoint.footprint == footprint_missed)
            // Don't touch entries installed with footprint_missed
            continue;
        return entry;
    }
    return NULL;
}

struct qcsched_entry *lookup_entry_by_address(CPUState *cpu, target_ulong inst)
{
    struct qcsched_schedpoint_window *window =
        &sched.schedpoint_window[cpu->cpu_index];

    if (window->from == END_OF_SCHEDPOINT_WINDOW)
        return NULL;

    for (int i = window->from; i < sched.total; i++) {
        struct qcsched_entry *entry = &sched.entries[i];
        if (cpu != NULL && entry->cpu != cpu->cpu_index)
            continue;
        if (entry->schedpoint.addr != inst)
            continue;
        return entry;
    }
    return NULL;
}

static int next_valid_order_of(CPUState *cpu, int order)
{
    struct qcsched_entry *entry = lookup_entry_by_order(NULL, order + 1);
    if (entry == NULL)
        return sched.total;
    else
        return entry->schedpoint.order;
}

static void
qcsched_window_activate_entry(CPUState *cpu,
                              struct qcsched_schedpoint_window *window,
                              struct qcsched_entry *entry)
{
    int err;

    ASSERT(!schedpoint_window_full(window), "CPU %d: Schedpoint window is full",
           cpu->cpu_index);

    if (entry->schedpoint.addr == QCSCHED_DUMMY_BREAKPOINT) {
        DRPRINTF(cpu, "Skip a dummy breakpoint on cpu#%d\n", entry->cpu);
        return;
    }

    if (entry->breakpoint.installed) {
        DRPRINTF(cpu, "[WARN] trying to actdivate the entry at %lx again\n",
                 entry->schedpoint.addr);
        return;
    }

    if (entry->schedpoint.footprint != footprint_preserved)
        DRPRINTF(cpu,
                 "[WARN] the footprint of the entry at %lx is not preserved. "
                 "Footprint: %d\n",
                 entry->schedpoint.addr, entry->schedpoint.footprint);

    DRPRINTF(cpu, "Installing a breakpoint at %lx on cpu#%d\n",
             entry->schedpoint.addr, entry->cpu);

    ASSERT(!(err = kvm_insert_breakpoint_cpu(cpu, entry->schedpoint.addr, 1,
                                             GDB_BREAKPOINT_HW)),
           "failed to insert a breakpiont at a scheduling point "
           "err=%d\n",
           err);

    entry->breakpoint.installed = true;

    window->activated++;
    DRPRINTF(cpu, "Window size after expand: %d\n", window->activated);
}

static void
qcsched_window_expand_window_1(CPUState *cpu,
                               struct qcsched_schedpoint_window *window)
{
    struct qcsched_entry *next,
        *entry = lookup_entry_by_order(cpu, window->until);

    if (!entry)
        // We are done with all breakpoints on this CPU
        return;

    qcsched_window_activate_entry(cpu, window, entry);

    next = lookup_entry_by_order(cpu, entry->schedpoint.order + 1);
    if (next != NULL)
        window->until = next->schedpoint.order;
    else
        window->until = END_OF_SCHEDPOINT_WINDOW;
}

void qcsched_window_expand_window_n(CPUState *cpu, int n)
{
    struct qcsched_schedpoint_window *window =
        &sched.schedpoint_window[cpu->cpu_index];

    for (int i = 0; i < n && !schedpoint_window_full(window); i++)
        qcsched_window_expand_window_1(cpu, window);
}

static void
qcsched_window_deactivate_entry_remote(CPUState *cpu,
                                       struct qcsched_schedpoint_window *window,
                                       struct qcsched_entry *entry)
{
    if (window->left_behind == END_OF_SCHEDPOINT_WINDOW ||
        window->left_behind > entry->schedpoint.order)
        window->left_behind = entry->schedpoint.order;
    // We do nothing here. The general breakpoint handler will handle
    // all left scheduling points.
}

static void
qcsched_window_deactivate_entry(CPUState *cpu,
                                struct qcsched_schedpoint_window *window,
                                struct qcsched_entry *entry)
{
    int err;

    ASSERT(window->cpu == entry->cpu,
           "window (%d) and entry (%d) have a different CPU index", window->cpu,
           entry->cpu);

    if (!entry->breakpoint.installed) {
        DRPRINTF(cpu,
                 "[WARN] trying to deactivate the entry at %lx that has not "
                 "been activated\n",
                 entry->schedpoint.addr);
        return;
    }

    ASSERT(!schedpoint_window_empty(window),
           "CPU %d: Schedpoint window is empty", cpu->cpu_index);

    DRPRINTF(cpu, "Removing a breakpoint at %lx on cpu#%d\n",
             entry->schedpoint.addr, entry->cpu);

    if (cpu->cpu_index != entry->cpu) {
        qcsched_window_deactivate_entry_remote(cpu, window, entry);
        return;
    }

    // NOTE: qcsched_handle_breakpoint_iolocked() always remove the
    // hit breakpoint so in this function -ENOENT is fine here
    err = kvm_remove_breakpoint_cpu(cpu, entry->schedpoint.addr, 1,
                                    GDB_BREAKPOINT_HW);
    ASSERT(!err || err == -ENOENT,
           "failed to remove a breakpiont at a scheduling point "
           "err=%d\n",
           err);

    entry->breakpoint.installed = false;

    window->activated--;
    DRPRINTF(cpu, "Window size after shrink: %d\n", window->activated);
}

static void
qcsched_window_shrink_entry(CPUState *cpu,
                            struct qcsched_schedpoint_window *window,
                            struct qcsched_entry *entry)
{
    struct qcsched_entry *next;
    CPUState *cpu0;

    ASSERT(window->cpu == entry->cpu,
           "window (%d) and entry (%d) have a different CPU index", window->cpu,
           entry->cpu);
    ASSERT(entry->schedpoint.order == window->from,
           "entry (%d) is not the first activated entry of the window (%d)",
           entry->schedpoint.order, window->from);

    if (entry != NULL && entry->breakpoint.installed)
        qcsched_window_deactivate_entry(cpu, window, entry);

    cpu0 = qemu_get_cpu(window->cpu);

    next = lookup_entry_by_order(cpu0, window->from + 1);
    if (next != NULL)
        window->from = next->schedpoint.order;
    else
        window->from = END_OF_SCHEDPOINT_WINDOW;

    if (window->from > window->until) {
        // NOTE: This can be possible, for example, if cpu0 does not
        // installed breakpoints yet, and cpu detects missed
        // schedpoint.
        next = lookup_entry_by_order(cpu0, window->from + 1);
        if (next != NULL)
            window->until = next->schedpoint.order;
        else
            window->until = END_OF_SCHEDPOINT_WINDOW;
    }
}

static void
qcsched_window_shrink_window_1(CPUState *cpu,
                               struct qcsched_schedpoint_window *window)
{
    struct qcsched_entry *entry = lookup_entry_by_order(cpu, window->from);

    if (!entry)
        // The window is already closed. Nothing to do.
        return;

    qcsched_window_shrink_entry(cpu, window, entry);
}

void qcsched_window_shrink_window_n(CPUState *cpu, int n)
{
    struct qcsched_schedpoint_window *window =
        &sched.schedpoint_window[cpu->cpu_index];

    for (int i = 0; i < n && !schedpoint_window_empty(window); i++)
        qcsched_window_shrink_window_1(cpu, window);
}

static void
qcsched_window_close_window(CPUState *cpu,
                            struct qcsched_schedpoint_window *window)
{
    int order;
    struct qcsched_entry *entry;
    DRPRINTF(cpu, "Closing the window\n");
    for (order = window->from; order < sched.total; order++) {
        entry = lookup_entry_by_order(cpu, order);
        if (entry == NULL)
            continue;
        if (entry->breakpoint.installed) {
            qcsched_window_leave_footprint_at(cpu, footprint_missed, order);
            qcsched_window_shrink_entry(cpu, window, entry);
        }
    }
    window->from = window->until = END_OF_SCHEDPOINT_WINDOW;
    ASSERT(window->activated == 0,
           "window still contains activated entries after closing");
}

void qcsched_window_prune_missed_schedpoint(CPUState *cpu)
{
    struct qcsched_schedpoint_window *window, *window0;
    struct qcsched_entry *hit, *legit, *entry;
    int order;

    window = &sched.schedpoint_window[cpu->cpu_index];

    hit = lookup_entry_by_address(cpu, cpu->regs.rip);
    legit = lookup_entry_by_order(cpu, window->from);

    if (legit == NULL || hit == NULL) {
        // There are two cases that legit or hit is NULL: 1) the
        // window is closed (i.e., window->from ==
        // END_OF_SCHEDPOINT_WINDOW), 2) another CPU resets the
        // schedule. For either case, we close the window, and abort
        // the schedule.
        qcsched_window_close_window(cpu, window);
        return;
    }

    ASSERT(hit->schedpoint.order >= window->from, "stale schedpoint");

    if (hit == legit)
        // We don't have missed schedpoints.
        return;

    DRPRINTF(cpu, "missing schedpoints [%d, %d)\n", legit->schedpoint.order,
             hit->schedpoint.order);

    // NOTE: hit will be deactivated later
    for (order = legit->schedpoint.order; order < hit->schedpoint.order;
         order++) {
        entry = lookup_entry_by_order(NULL, order);
        ASSERT(entry, "entry should not be NULL. order=%d", order);

        window0 = &sched.schedpoint_window[entry->cpu];

        qcsched_window_leave_footprint_at(cpu, footprint_missed,
                                          entry->schedpoint.order);
        qcsched_window_shrink_entry(cpu, window0, entry);
    }
}

void qcsched_window_cleanup_left_schedpoint(CPUState *cpu)
{
    int i;
    struct qcsched_entry *entry, *next;
    struct qcsched_schedpoint_window *window =
        &sched.schedpoint_window[cpu->cpu_index];

    ASSERT(qemu_mutex_iothread_locked(), "iothread mutex is not locked");

    for (i = window->left_behind; i < window->from;) {
        entry = lookup_entry_by_order(cpu, i);
        if (entry == NULL)
            break;
        if (entry->breakpoint.installed) {
            DRPRINTF(cpu, "Cleanup a schedpoint at %lx\n",
                     entry->schedpoint.addr);
            qcsched_window_leave_footprint_at(cpu, footprint_dropped, i);
            qcsched_window_deactivate_entry(cpu, window, entry);
        }
        next = lookup_entry_by_order(cpu, entry->schedpoint.order + 1);
        if (!next)
            break;
        i = next->schedpoint.order;
    }
    // We don't touch window->left_behind when expanding the window,
    // so we should set left->behind to the end of schedpoint window.
    window->left_behind = END_OF_SCHEDPOINT_WINDOW;
}

void qcsched_window_sync(CPUState *cpu)

{
    int left_behind;
    struct qcsched_entry *entry;
    struct qcsched_schedpoint_window *window;

    window = &sched.schedpoint_window[cpu->cpu_index];

    if (sched.current <= window->from)
        return;

    // The focus of the schedule has moved forward so the current
    // window is stale. Let's synchronize the window.

    left_behind = window->from;
    entry = lookup_entry_by_order(cpu, sched.current);
    if (entry == NULL)
        window->from = END_OF_SCHEDPOINT_WINDOW;
    else
        window->from = entry->schedpoint.order;

    if (window->until < window->from)
        window->until = window->from;

    if (left_behind < window->left_behind)
        window->left_behind = left_behind;
}

bool qcsched_window_hit_stale_schedpoint(CPUState *cpu)
{
    struct qcsched_schedpoint_window *window =
        &sched.schedpoint_window[cpu->cpu_index];
    struct qcsched_entry *hit = lookup_entry_by_address(cpu, cpu->regs.rip);
    if (hit == NULL)
        // Cannot find the entry. Possibly sched is reset (i.e.,
        // sched.total == 0) or the window is closed (i.e.,
        // window->from == END_OF_SCHEDPOINT_WINDOW). Either cases
        // mean that the breakpoint is stale.
        return true;
    return hit->schedpoint.order < window->from;
}

void forward_focus(CPUState *cpu, int step)
{
    int current = sched.current + step;
    enum qcschedpoint_footprint footprint =
        sched.entries[current].schedpoint.footprint;

    if (footprint)
        DRPRINTF(
            cpu,
            "[WARN] moving the focus to an invalid entry: %d (footprint %d)",
            current, footprint);
    sched.current = current;

    DRPRINTF(cpu, "Next scheduling point: %d, %lx\n", current,
             sched.entries[current].schedpoint.addr);
}

void hand_over_baton_from(CPUState *cpu, int order)
{
    int next_order = next_valid_order_of(cpu, order);
    forward_focus(cpu, next_order - sched.current);
}

bool qcsched_window_lock_contending(CPUState *cpu)
{
    CPUState *next_cpu;
    bool contending;
    struct qcsched_entry *entry =
        lookup_entry_by_order(NULL, sched.current + 1);

    // Allowed: activated
    if (!qcsched_check_cpu_state(cpu, qcsched_cpu_activated) ||
        qcsched_check_cpu_state(cpu, qcsched_cpu_deactivated))
        return false;

    if (!entry) {
        // XXX: Although at this point we support only two CPUs so
        // this makes sense, the problem is that we don't have a
        // mechanism to determine the next CPU after the last
        // scheduling point. Need to fix it.
        bool found = false;
        CPU_FOREACH(next_cpu)
        {
            if (next_cpu != cpu &&
                sched.cpu_state[next_cpu->cpu_index] == qcsched_cpu_activated) {
                found = true;
                break;
            }
        }
        if (!found)
            return false;
    } else {
        next_cpu = qemu_get_cpu(entry->cpu);
    }

    contending = qcsched_vmi_lock_contending(cpu, next_cpu);
    if (contending)
        DRPRINTF(cpu, "Contending on a lock\n");
    return contending;
}

bool qcsched_window_consecutive_schedpoint(CPUState *cpu, int current_order)
{
    struct qcsched_entry *entry;
    int next_order = next_valid_order_of(cpu, current_order);

    if (next_order == sched.total)
        // We reach the end of the schedule window
        return true;

    entry = lookup_entry_by_order(NULL, next_order);
    if (entry == NULL)
        // Schedule is reset. Just let this CPU keep executing.
        return true;

    return entry->cpu == cpu->cpu_index;
}

void qcsched_window_leave_footprint_at(CPUState *cpu,
                                       enum qcschedpoint_footprint footprint,
                                       int order)
{
    // We don't use lookup_entry_by_order() as we want an entry
    // exactly at order
    struct qcsched_entry *entry;

    if (order >= sched.total)
        // This is possible for example one clears breakpoints and the
        // other deacitavtes late.
        return;

    entry = &sched.entries[order];
    if (entry->cpu != cpu->cpu_index)
        return;

    if (entry->schedpoint.footprint != footprint_preserved)
        DRPRINTF(cpu, "[WARN] footprint is already left\n");
#ifdef _DEBUG_VERBOSE
    DRPRINTF(cpu, "Leave footprint %d at an entry #%d\n", footprint, order);
#endif
    entry->schedpoint.footprint = footprint;
}

void qcsched_window_leave_footprint(CPUState *cpu,
                                    enum qcschedpoint_footprint footprint)
{
    struct qcsched_schedpoint_window *window =
        &sched.schedpoint_window[cpu->cpu_index];
    qcsched_window_leave_footprint_at(cpu, footprint, window->from);
}
