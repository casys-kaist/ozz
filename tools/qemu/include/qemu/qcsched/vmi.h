#ifndef __QCSCHED_VMI_H
#define __QCSCHED_VMI_H

#include "qemu/osdep.h"
#include "cpu.h"

struct qcsched_vmi_info {
    target_ulong trampoline_addr;
    target_ulong hook_addr;
};

struct qcsched_vmi_task {
    target_ulong stack;
};

extern struct qcsched_vmi_info vmi_info;

void qcsched_vmi_set_trampoline(CPUState *cpu, target_ulong addr);
void qcsched_vmi_set_hook(CPUState *cpu, target_ulong addr);

void qcsched_vmi_task(CPUState *cpu, struct qcsched_vmi_task *t);
bool qcsched_vmi_can_progress(CPUState *cpu);

#endif
