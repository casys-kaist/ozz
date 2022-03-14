#define _DEBUG

#include "qemu/osdep.h"

#include "cpu.h"
#include "sysemu/kvm.h"

#include "qemu/qcsched/qcsched.h"
#include "qemu/qcsched/window.h"

#define schedpoint_window_full(window)                                         \
    (window->activated == SCHEDPOINT_WINDOW_SIZE)
#define schedpoint_window_empty(window) (window->activated == 0)

static struct qcsched_entry *
qcsched_window_lookup_entry(CPUState *cpu,
                            struct qcsched_schedpoint_window *window, int from)
{
    if (from == END_OF_SCHEDPOINT_WINDOW)
        return NULL;
    for (int i = from; i < sched.total; i++) {
        struct qcsched_entry *entry = &sched.entries[i];
        if (entry->cpu != cpu->cpu_index)
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
        *entry = qcsched_window_lookup_entry(cpu, window, window->until);

    if (!entry)
        // We are done with all breakpoints on this CPU
        return;

    qcsched_window_activate_entry(cpu, window, entry);

    next =
        qcsched_window_lookup_entry(cpu, window, entry->schedpoint.order + 1);

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
qcsched_window_deactivate_entry(CPUState *cpu,
                                struct qcsched_schedpoint_window *window,
                                struct qcsched_entry *entry)
{
    int err;

    ASSERT(!schedpoint_window_empty(window), "Schedpoint window is empty");

    if (!entry->breakpoint.installed) {
        DRPRINTF(cpu,
                 "[WARN] trying to deactivate the entry at %lx that has not "
                 "been activated\n",
                 entry->schedpoint.addr);
        return;
    }

    DRPRINTF(cpu, "Removing a breakpoint at %lx on cpu#%d\n",
             entry->schedpoint.addr, entry->cpu);

    // XXX: qcsched_handle_breakpoint_iolocked() always remove the hit
    // breakpoint so in this function the breakpoint is always
    // removed. Although I think we can safely remove below
    // statements, leave they there just in case.
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
qcsched_window_shrink_window_1(CPUState *cpu,
                               struct qcsched_schedpoint_window *window)
{
    struct qcsched_entry *next,
        *entry = qcsched_window_lookup_entry(cpu, window, window->from);

    if (entry != NULL)
        qcsched_window_deactivate_entry(cpu, window, entry);

    next = qcsched_window_lookup_entry(cpu, window, window->from + 1);
    if (next != NULL)
        window->from = next->schedpoint.order;
    else
        window->from = END_OF_SCHEDPOINT_WINDOW;
}

void qcsched_window_shrink_window_n(CPUState *cpu, int n)
{
    struct qcsched_schedpoint_window *window =
        &sched.schedpoint_window[cpu->cpu_index];

    for (int i = 0; i < n && !schedpoint_window_empty(window); i++)
        qcsched_window_shrink_window_1(cpu, window);
}
