#ifndef __QCSCHED_H
#define __QCSCHED_H

#include "qemu/osdep.h"

#include "cpu.h"
#include "exec/gdbstub.h"

#include "qemu/qcsched/vmi.h"

#define MAX_SCHEDPOINTS 8

struct qcschedpoint {
    target_ulong addr;
    int order;
};

struct qcsched_breakpoint {
    struct qcschedpoint *schedpoint;
    bool installed;
    bool suspended;
};

struct qcsched_entry {
    struct qcschedpoint schedpoint;
    struct qcsched_breakpoint breakpoint;
    struct qcsched_vmi_task t;
    int cpu;
};

struct qcsched {
    struct qcsched_entry entries[MAX_SCHEDPOINTS];
    int total, current;
    bool activated;
};

void qcsched_pre_run(CPUState *cpu);
void qcsched_post_run(CPUState *cpu);
void qcsched_commit_state(CPUState *cpu, target_ulong hcall_ret);

void qcsched_handle_hcall(CPUState *cpu, struct kvm_run *run);
int qcsched_handle_breakpoint(CPUState *cpu);

extern struct qcsched sched;

void qcsched_escape_if_trampoled(CPUState *cpu, CPUState *wakeup);

#ifdef _DEBUG
#define DRPRINTF(cpu, fmt, ...) fprintf(stderr, "[CPU #%d] " fmt, cpu->cpu_index, ## __VA_ARGS__)
#else
#define DRPRINTF(cpu, fmt, ...) do { } while(0)
#endif

#define ASSERT(cond, fmt, ...)                          \
    do {                                                \
        if (!(cond)) {                                  \
            fprintf(stderr, fmt "\n", ##__VA_ARGS__);   \
            exit(1);                                    \
        }                                               \
    } while(0);

#endif
