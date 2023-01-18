#define _DEBUG

#include "qemu/osdep.h"

#include "cpu.h"

#include "qemu/qcsched/exec_control.h"

bool blocker_task_kidnapped(CPUState *cpu) { return false; }

void blocker_kidnap_task(CPUState *cpu) {}

void blocker_resume_task(CPUState *cpu) {}
