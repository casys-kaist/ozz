#ifndef __QCSCHED_H
#define __QCSCHED_H

#include "qemu/osdep.h"

#include "cpu.h"
#include "exec/gdbstub.h"

#define MAX_SCHEDPOINTS 8

struct qcsched_vmi_task {
};

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
    struct qcsched_vmi_task *t;
    int cpu;
};

struct qcsched {
    struct qcsched_entry entries[MAX_SCHEDPOINTS];
    int total;
    bool activated;
};

void qcsched_pre_run(CPUState *cpu);
void qcsched_post_run(CPUState *cpu);
void qcsched_commit_state(CPUState *cpu, target_ulong hcall_ret);

void qcsched_handle_hcall(CPUState *cpu, struct kvm_run *run);
int qcsched_handle_breakpoint(CPUState *cpu);

#ifdef _DEBUG
#define DRPRINTF(fmt, ...) fprintf(stderr, fmt, __VA_ARGS__)
#else
#define DRPRINTF(fmt, ...) do { } while(0)
#endif

#endif
