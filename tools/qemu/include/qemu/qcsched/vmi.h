#ifndef __QCSCHED_VMI_H
#define __QCSCHED_VMI_H

#include "cpu.h"
#include "qemu/osdep.h"
#include "qemu/qcsched/constant.h"

#define MAX_LOCKS 128

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

struct qcsched_vmi_info {
    target_ulong trampoline_addr[2];
#define trampoline_entry_addr trampoline_addr[0]
#define trampoline_exit_addr trampoline_addr[1]
    target_ulong hook_addr;
    target_ulong __per_cpu_offset[64];
    target_ulong current_task;
    target_ulong __ssb_do_emulate;
    struct qcsched_vmi_lock_info lock_info[MAX_CPUS];
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

bool vmi_same_task(struct qcsched_vmi_task *t0, struct qcsched_vmi_task *t1);

#endif
