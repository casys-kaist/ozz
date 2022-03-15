#ifndef __WINDOW_H
#define __WINDOW_H

#include "qemu/osdep.h"

#include "cpu.h"

#ifdef CONFIG_QCSCHED

// The maximum size of a scheduling window is (the number of hardware
// breakpoints - 1 (dedicated for escaping the trampoline).
#define SCHEDPOINT_WINDOW_SIZE 3

// NOTE: We have scheduling points more than hardware breakpoints so
// that we cannot install breakpoints on all scheduling points at a
// time. If the number of scheduling points is larger than the number
// of hardware breakpoints, we window the scheduling points.
struct qcsched_schedpoint_window {
    int total;
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

void qcsched_window_prune_passed_schedpoint(CPUState *);
void qcsched_window_cleanup_left_schedpoint(CPUState *);

void qcsched_window_sync(CPUState *);
bool qcsched_window_hit_stale_schedpoint(CPUState *);

void forward_focus(CPUState *cpu, int step);
#define hand_over_baton(cpu) forward_focus(cpu, 1)

#else

void qcsched_window_expand_window_n(CPUState *, int) {}
void qcsched_window_shrink_window_n(CPUState *, int) {}
void qcsched_window_expand_window(CPUState *) {}
void qcsched_window_shrink_window(CPUState *) {}
void qcsched_window_prune_passed_schedpoint(CPUState *) {}
void qcsched_window_cleanup_left_schedpoint(CPUState *) {}
void forward_focus(CPUState *cpu, int step) {}
void hand_over_baton(cpu) {}
void qcsched_window_sync(CPUState *) {}
bool qcsched_window_hit_stale_schedpoint(CPUState *) {}

#endif /* CONFIG_QCSCHED */

#endif /* __WINDOW_H */
