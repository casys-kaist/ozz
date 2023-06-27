#ifndef __QCSCHED_VMI_H
#define __QCSCHED_VMI_H

#include "cpu.h"
#include "qemu/osdep.h"
#include "qemu/qcsched/constant.h"

#define PREEMPT_BITS 8
#define SOFTIRQ_BITS 8
#define HARDIRQ_BITS 4
#define NMI_BITS 4

#define PREEMPT_SHIFT 0
#define SOFTIRQ_SHIFT (PREEMPT_SHIFT + PREEMPT_BITS)
#define HARDIRQ_SHIFT (SOFTIRQ_SHIFT + SOFTIRQ_BITS)
#define NMI_SHIFT (HARDIRQ_SHIFT + HARDIRQ_BITS)

#define __IRQ_MASK(x) ((1UL << (x)) - 1)

#define PREEMPT_MASK (__IRQ_MASK(PREEMPT_BITS) << PREEMPT_SHIFT)
#define SOFTIRQ_MASK (__IRQ_MASK(SOFTIRQ_BITS) << SOFTIRQ_SHIFT)
#define HARDIRQ_MASK (__IRQ_MASK(HARDIRQ_BITS) << HARDIRQ_SHIFT)
#define NMI_MASK (__IRQ_MASK(NMI_BITS) << NMI_SHIFT)

#define PREEMPT_OFFSET (1UL << PREEMPT_SHIFT)
#define SOFTIRQ_OFFSET (1UL << SOFTIRQ_SHIFT)
#define HARDIRQ_OFFSET (1UL << HARDIRQ_SHIFT)
#define NMI_OFFSET (1UL << NMI_SHIFT)

#define MAX_LOCKS 128
#define MAX_WHITELIST_ITEM 16

struct qcsched_vmi_lock {
    target_ulong lockdep_addr;
    target_ulong ip;
    int trylock;
    int read;
};

struct qcsched_vmi_lock_info {
    int count;
    struct qcsched_vmi_lock acquired[MAX_LOCKS];
};

struct qcsched_lockdep_whitelist {
    int count;
    target_ulong whitelist[MAX_WHITELIST_ITEM];
};

struct qcsched_vmi_info {
    target_ulong trampoline_addr[2];
#define trampoline_entry_addr trampoline_addr[0]
#define trampoline_exit_addr trampoline_addr[1]
    target_ulong hook_addr;
    target_ulong __per_cpu_offset[64];
    target_ulong current_task;
    target_ulong __ssb_do_emulate;
    target_ulong __preempt_count;
    struct qcsched_vmi_lock_info lock_info[MAX_CPUS];
    struct qcsched_lockdep_whitelist lockdep_whitelist;
};

struct qcsched_vmi_task {
    target_ulong task_struct;
};

extern struct qcsched_vmi_info vmi_info;

target_ulong qcsched_vmi_hint(CPUState *cpu, target_ulong type,
                              target_ulong addr, target_ulong misc);
void qcsched_vmi_lock_info_reset(CPUState *cpu);

void qcsched_vmi_task(CPUState *cpu, struct qcsched_vmi_task *t);
bool qcsched_vmi_can_progress(CPUState *cpu);
bool qcsched_vmi_lock_contending(CPUState *, CPUState *);
bool qcsched_vmi_in_task(CPUState *cpu);

bool vmi_same_task(struct qcsched_vmi_task *t0, struct qcsched_vmi_task *t1);

bool qcsched_vmi_running_context_being_scheduled(CPUState *cpu, bool task_only);

#endif
