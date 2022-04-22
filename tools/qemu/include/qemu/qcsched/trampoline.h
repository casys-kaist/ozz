#ifndef __TRAMPOLINE_H
#define __TRAMPOLINE_H

#include "qemu/osdep.h"

#include "cpu.h"

bool task_kidnapped(CPUState *cpu);
void kidnap_task(CPUState *cpu);
void resume_task(CPUState *cpu);
void wake_cpu_up(CPUState *cpu, CPUState *wakeup);
void wake_others_up(CPUState *cpu);
void qcsched_escape_if_trampoled(CPUState *cpu, CPUState *wakeup);
struct qcsched_trampoline_info *get_trampoline_info(CPUState *cpu);

#endif /* __TRAMPOLINE_H */
