#ifndef __QCSCHED_VMI_H
#define __QCSCHED_VMI_H

#include "qemu/osdep.h"
#include "cpu.h"

struct qcsched_vmi_info {
    target_ulong trampoline_addr[2];
#define trampoline_entry_addr trampoline_addr[0]
#define trampoline_exit_addr trampoline_addr[1]
    target_ulong hook_addr;
    target_ulong __per_cpu_offset[64];
    target_ulong current_task;
    target_ulong __ssb_do_emulate;
};

struct qcsched_vmi_task {
    target_ulong task_struct;
};

extern struct qcsched_vmi_info vmi_info;

target_ulong qcsched_vmi_hint(CPUState *cpu, target_ulong type, target_ulong addr);

void qcsched_vmi_task(CPUState *cpu, struct qcsched_vmi_task *t);
bool qcsched_vmi_can_progress(CPUState *cpu);
bool vmi_same_task(struct qcsched_vmi_task *t0,
                   struct qcsched_vmi_task *t1);

#endif
