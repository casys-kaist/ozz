#ifndef __QCSCHED_H
#define __QCSCHED_H

#include "qemu/osdep.h"

#include "cpu.h"
#include "exec/gdbstub.h"

#include "qemu/qcsched/vmi.h"
#include "qemu/qcsched/window.h"

#define MAX_SCHEDPOINTS 8
// TODO: Do not use macro
#define MAX_CPUS 8

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

struct qcsched_breakpoint_record {
    target_ulong RIP;
    int count;
};

#define WATCHDOG_BREAKPOINT_COUNT_MAX 10

struct qcsched {
    struct qcsched_entry entries[MAX_SCHEDPOINTS];
    struct qcsched_breakpoint_record last_breakpoint[MAX_CPUS];
    struct qcsched_schedpoint_window schedpoint_window[MAX_CPUS];
    int total, current;
    bool activated;
    bool used;
};

void qcsched_init_vcpu(CPUState *cpu);

void qcsched_pre_run(CPUState *cpu);
void qcsched_post_run(CPUState *cpu);
void qcsched_commit_state(CPUState *cpu, target_ulong hcall_ret);

void qcsched_handle_hcall(CPUState *cpu, struct kvm_run *run);
int qcsched_handle_breakpoint(CPUState *cpu);

extern struct qcsched sched;

struct qcsched_trampoline_info {
    struct qcsched_vmi_task t;
    struct kvm_regs orig_regs;
    bool trampoled;
    bool kicked;
    // timerid should be the last member because of resume_task().
    timer_t timerid;
};

void wake_cpu_up(CPUState *cpu, CPUState *wakeup);
void wake_others_up(CPUState *cpu);

void qcsched_arm_selfescape_timer(CPUState *cpu);
void qcsched_escape_if_trampoled(CPUState *cpu, CPUState *wakeup);
struct qcsched_trampoline_info *get_trampoline_info(CPUState *cpu);
void qcsched_handle_kick(CPUState *cpu);

#ifdef _DEBUG
#define DRPRINTF(cpu, fmt, ...)                                                \
    fprintf(stderr, "[CPU #%d] " fmt, cpu->cpu_index, ##__VA_ARGS__)
#else
#define DRPRINTF(cpu, fmt, ...)                                                \
    do {                                                                       \
    } while (0)
#endif

#define ASSERT(cond, fmt, ...)                                                 \
    do {                                                                       \
        if (!(cond)) {                                                         \
            fprintf(stderr, fmt "\n", ##__VA_ARGS__);                          \
            exit(1);                                                           \
        }                                                                      \
    } while (0);

#define TRAMPOLINE_ESCAPE_MAGIC 0x75da1791

#endif
