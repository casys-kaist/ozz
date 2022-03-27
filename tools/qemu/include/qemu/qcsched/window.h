#ifndef __WINDOW_H
#define __WINDOW_H

#include "qemu/osdep.h"

#include "cpu.h"

#ifdef CONFIG_QCSCHED

#include "qemu/qcsched/constant.h"

// The maximum size of a scheduling window is (the number of hardware
// breakpoints - 1 (dedicated for escaping the trampoline).
#define SCHEDPOINT_WINDOW_SIZE 3

// NOTE: We have scheduling points more than hardware breakpoints so
// that we cannot install breakpoints on all scheduling points at a
// time. If the number of scheduling points is larger than the number
// of hardware breakpoints, we window the scheduling points.
struct qcsched_schedpoint_window {
    int total;
    // This indicates the number of missed schedpoints of "previous"
    // run.
    int missed_schedpoint;
    int activated;
    // from is the order of a breakpoint that is installed and will be
    // hit first. until is the order of a next breakpoint of the last
    // one in the window. I.e., on this CPU, a window contains
    // scheduling points with an order ranging [from, until) and their
    // dedicated cpu is this one.
    int from;
    int until;
    int cpu;
    // When a CPU detects missed scheduling points, it removes
    // breakpoints itself for the breakpoints installed on it, or
    // defers the removing job to a corresponding CPU. the schedpoint
    // window [left_behind, from) represents scheduling points that
    // are deferred so will be removed later.
    int left_behind;
};

void qcsched_window_expand_window_n(CPUState *, int);
void qcsched_window_shrink_window_n(CPUState *, int);

#define qcsched_window_expand_window(cpu)                                      \
    qcsched_window_expand_window_n(cpu, SCHEDPOINT_WINDOW_SIZE)
#define qcsched_window_shrink_window(cpu) qcsched_window_shrink_window_n(cpu, 1)

void qcsched_window_prune_missed_schedpoint(CPUState *);
void qcsched_window_cleanup_left_schedpoint(CPUState *);

void qcsched_window_sync(CPUState *);
bool qcsched_window_hit_stale_schedpoint(CPUState *);

bool qcsched_window_lock_contending(CPUState *);
bool qcsched_window_consecutive_schedpoint(CPUState *cpu, int order);

void forward_focus(CPUState *cpu, int step);
void hand_over_baton_from(CPUState *cpu, int order);

void qcsched_window_leave_footprint(CPUState *cpu,
                                    enum qcschedpoint_footprint footprint);
void qcsched_window_leave_footprint_at(CPUState *cpu,
                                       enum qcschedpoint_footprint footprint,
                                       int order);

struct qcsched_entry *lookup_entry_by_order(CPUState *cpu, int from);
struct qcsched_entry *lookup_entry_by_address(CPUState *cpu, target_ulong inst);

#else

void qcsched_window_expand_window_n(CPUState *, int) {}
void qcsched_window_shrink_window_n(CPUState *, int) {}
void qcsched_window_expand_window(CPUState *) {}
void qcsched_window_shrink_window(CPUState *) {}
void qcsched_window_prune_missed_schedpoint(CPUState *) {}
void qcsched_window_cleanup_left_schedpoint(CPUState *) {}
void forward_focus(CPUState *cpu, int step) {}
void hand_over_baton(CPUState *cpu) {}
void qcsched_window_sync(CPUState *) {}
bool qcsched_window_hit_stale_schedpoint(CPUState *) {}
bool qcsched_window_lock_contending(CPUState *) {}
bool qcsched_window_consecutive_schedpoint(CPUState *cpu) {}
void qcsched_window_leave_footprint(CPUState *cpu,
                                    enum qcschedpoint_footprint footprint)
{
}
void qcsched_window_leave_footprint_at(CPUState *cpu,
                                       enum qcschedpoint_footprint footprint,
                                       int order)
{
}
struct qcsched_entry *lookup_entry_by_order(CPUState *, int) {}
struct qcsched_entry *lookup_entry_by_address(CPUState *, target_ulong) {}

#endif /* CONFIG_QCSCHED */

#endif /* __WINDOW_H */
