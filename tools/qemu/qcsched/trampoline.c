#define _DEBUG

#include "qemu/osdep.h"

#include "exec/gdbstub.h"
#include "qemu/main-loop.h"
#include "sysemu/kvm.h"

#include "qemu/qcsched/exec_control.h"
#include "qemu/qcsched/qcsched.h"
#include "qemu/qcsched/vmi.h"
#include "qemu/qcsched/window.h"

static struct qcsched_trampoline_info *get_trampoline_info(CPUState *cpu)
{
    struct qcsched_exec_info *info = get_exec_info(cpu);
    return &info->trampoline;
}

static void jump_into_trampoline(CPUState *cpu)
{
    RIP(cpu) = vmi_info.trampoline_entry_addr;
    cpu->qcsched_dirty = true;
}

static void __copy_registers(struct kvm_regs *dst, struct kvm_regs *src)
{
    *dst = *src;
}

void trampoline_kidnap_task(CPUState *cpu)
{
    struct qcsched_trampoline_info *trampoline = get_trampoline_info(cpu);

    __copy_registers(&trampoline->orig_regs, &cpu->regs);
    jump_into_trampoline(cpu);
    trampoline->trampoled = true;
}

void trampoline_resume_task(CPUState *cpu)
{
    struct qcsched_trampoline_info *trampoline = get_trampoline_info(cpu);
    __copy_registers(&cpu->regs, &trampoline->orig_regs);
    cpu->qcsched_dirty = true;
    cpu->qcsched_force_wakeup = false;
    memset(trampoline, 0, sizeof(*trampoline));
    // XXX: I'm not sure info->kicked should be reset. I just follow
    // the previous implementation.
    struct qcsched_exec_info *info = (struct qcsched_exec_info *)trampoline;
    info->kicked = false;
}

bool trampoline_task_kidnapped(CPUState *cpu)
{
    struct qcsched_trampoline_info *trampoline = get_trampoline_info(cpu);
    return trampoline->trampoled;
}
