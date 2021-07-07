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
};

struct qcsched_vmi_task {
    target_ulong task_struct;
};

extern struct qcsched_vmi_info vmi_info;

void qcsched_vmi_set_trampoline(CPUState *cpu, target_ulong addr, int index);
void qcsched_vmi_set_hook(CPUState *cpu, target_ulong addr);
void qcsched_vmi_set_current_task(CPUState *cpu, target_ulong addr);
void qcsched_vmi_set__per_cpu_offset(CPUState *cpu, int index, target_ulong addr);

void qcsched_vmi_task(CPUState *cpu, struct qcsched_vmi_task *t);
bool qcsched_vmi_can_progress(CPUState *cpu);

#endif
