#ifndef __TRAMPOLINE_H
#define __TRAMPOLINE_H

#include "qemu/osdep.h"

#include "cpu.h"

#include "qemu/qcsched/vmi.h"

struct qcsched_trampoline_info {
    struct qcsched_vmi_task t;
    struct kvm_regs orig_regs;
    bool trampoled;
};

void trampoline_kidnap_task(CPUState *cpu);
void trampoline_resume_task(CPUState *cpu);
bool trampoline_task_kidnapped(CPUState *cpu);

#endif /* __TRAMPOLINE_H */
