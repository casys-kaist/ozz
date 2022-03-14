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

static struct qcsched_entry *lookup_entry_by_order(CPUState *cpu, int from)
{
    if (from == END_OF_SCHEDPOINT_WINDOW)
        return NULL;
    for (int i = from; i < sched.total; i++) {
        struct qcsched_entry *entry = &sched.entries[i];
        if (cpu != NULL && entry->cpu != cpu->cpu_index)
            continue;
        return entry;
    }
    return NULL;
}

static struct qcsched_entry *lookup_entry_by_address(CPUState *cpu,
                                                     target_ulong inst)
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

static void
qcsched_window_activate_entry(CPUState *cpu,
                              struct qcsched_schedpoint_window *window,
                              struct qcsched_entry *entry)
{
    int err;

    ASSERT(!schedpoint_window_full(window), "Schedpoint window is full");

    if (entry->schedpoint.addr == QCSCHED_DUMMY_BREAKPOINT) {
        DRPRINTF(cpu, "Skip a dummy breakpoint on cpu#%d\n", entry->cpu);
        return;
    }

    if (entry->breakpoint.installed) {
        DRPRINTF(cpu, "[WARN] trying to actdivate the entry at %lx again\n",
                 entry->schedpoint.addr);
        return;
    }

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

    ASSERT(!schedpoint_window_empty(window), "Schedpoint window is empty");
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

    if (entry != NULL)
        qcsched_window_deactivate_entry(cpu, window, entry);

    cpu0 = qemu_get_cpu(window->cpu);

    next = lookup_entry_by_order(cpu0, window->from + 1);
    if (next != NULL)
        window->from = next->schedpoint.order;
    else
        window->from = END_OF_SCHEDPOINT_WINDOW;
}

static void
qcsched_window_shrink_window_1(CPUState *cpu,
                               struct qcsched_schedpoint_window *window)
{
    struct qcsched_entry *entry = lookup_entry_by_order(cpu, window->from);

    qcsched_window_shrink_entry(cpu, window, entry);
}

void qcsched_window_shrink_window_n(CPUState *cpu, int n)
{
    struct qcsched_schedpoint_window *window =
        &sched.schedpoint_window[cpu->cpu_index];

    for (int i = 0; i < n && !schedpoint_window_empty(window); i++)
        qcsched_window_shrink_window_1(cpu, window);
}

void qcsched_window_prune_passed_schedpoint(CPUState *cpu)
{
    struct qcsched_schedpoint_window *window, *window0;
    struct qcsched_entry *hit, *legit, *entry;
    int order, missed;

    window = &sched.schedpoint_window[cpu->cpu_index];

    hit = lookup_entry_by_address(cpu, cpu->regs.rip);
    legit = lookup_entry_by_order(cpu, window->from);

    ASSERT(legit == NULL || legit->schedpoint.order >= sched.current,
           "the legitimate schedpoint (%d) is behind the current focus (%d)",
           legit->schedpoint.order, sched.current);

    if (hit == legit)
        // We don't have missed schedpoints.
        return;

    missed = hit->schedpoint.order - legit->schedpoint.order;

    // NOTE: hit will be deactivated later
    for (order = legit->schedpoint.order; order < hit->schedpoint.order;
         order++) {
        entry = lookup_entry_by_order(NULL, order);
        ASSERT(entry, "entry should not be NULL. order=%d", order);

        window0 = &sched.schedpoint_window[entry->cpu];

        qcsched_window_shrink_entry(cpu, window0, entry);
    }

    forward_focus(cpu, missed);
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
        DRPRINTF(cpu, "Cleanup a schedpoint at %lx\n", entry->schedpoint.addr);
        qcsched_window_deactivate_entry(cpu, window, entry);
        next = lookup_entry_by_order(cpu, entry->schedpoint.order + 1);
        i = next->schedpoint.order;
    }
    // We don't touch window->left_behind when expanding the window,
    // so we should set left->behind to the end of schedpoint window.
    window->left_behind = END_OF_SCHEDPOINT_WINDOW;
}

void forward_focus(CPUState *cpu, int step)
{
    sched.current = sched.current + step;
    DRPRINTF(cpu, "Next scheduling point: %d, %lx\n", sched.current,
             sched.entries[sched.current].schedpoint.addr);
}
