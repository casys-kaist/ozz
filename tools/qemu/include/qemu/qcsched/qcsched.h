#ifndef __QCSCHED_H
#define __QCSCHED_H

#include "qemu/osdep.h"

#include "cpu.h"
#include "exec/gdbstub.h"

#include "qemu/qcsched/constant.h"
#include "qemu/qcsched/state.h"
#include "qemu/qcsched/vmi.h"
#include "qemu/qcsched/window.h"

struct qcschedpoint {
    target_ulong addr;
    int order;
    enum qcschedpoint_footprint footprint;
};

struct qcsched_breakpoint {
    struct qcschedpoint *schedpoint;
    bool installed;
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

struct qcsched {
    struct qcsched_entry entries[MAX_SCHEDPOINTS];
    struct qcsched_breakpoint_record last_breakpoint[MAX_CPUS];
    struct qcsched_schedpoint_window schedpoint_window[MAX_CPUS];
    enum qcsched_cpu_state cpu_state[MAX_CPUS];
    int total, current;
    int orig_nr_bps;
    int nr_cpus;
    bool kssb_enabled; // indicating kssb is enabled in the kernel
    bool activated;    // indicating the schedule is activated in QEMU
    bool used;
};

void qcsched_init_vcpu(CPUState *cpu);

bool qcsched_pre_run(CPUState *cpu);
void qcsched_post_run(CPUState *cpu);
void qcsched_yield_turn_from(CPUState *cpu, int order);
void qcsched_keep_this_cpu_going(CPUState *cpu);

void qcsched_handle_hcall(CPUState *cpu, struct kvm_run *run);
int qcsched_handle_breakpoint(CPUState *cpu);

enum {
    warn_once_task_kidnapped,
    warn_once_total,
};

extern bool warn_once[warn_once_total];

extern struct qcsched sched;

void qcsched_arm_selfescape_timer(CPUState *cpu);
void qcsched_handle_kick(CPUState *cpu);

enum qcsched_cookie {
    cookie_hcall,
    cookie_breakpoint,
    cookie_timer,
};

void qcsched_eat_cookie(CPUState *cpu, enum qcsched_cookie type);

#define RIP(cpu) (cpu->regs.rip)

#ifdef _DEBUG
#define DRPRINTF(cpu, fmt, ...)                                                \
    fprintf(stderr, "[CPU #%d-%llu/%llu/%llu] " fmt, cpu->cpu_index,           \
            cpu->hcall_cookie, cpu->breakpoint_cookie, cpu->timer_cookie,      \
            ##__VA_ARGS__)
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

#endif
