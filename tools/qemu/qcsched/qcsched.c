#include "qemu/osdep.h"

#include <linux/kvm.h>

#include "cpu.h"
#include "sysemu/kvm.h"

#include "qemu/qcsched/qcsched.h"

struct qcsched sched;

void qcsched_pre_run(CPUState *cpu) {}

void qcsched_post_run(CPUState *cpu) { kvm_read_registers(cpu, &cpu->regs); }

static void qcsched_skip_executed_vmcall(struct kvm_regs *regs)
{
#define VMCALL_INSN_LEN 3
    regs->rip += VMCALL_INSN_LEN;
}

void qcsched_commit_state(CPUState *cpu, target_ulong hcall_ret)
{
    qcsched_skip_executed_vmcall(&cpu->regs);
    cpu->regs.rax = hcall_ret;
    kvm_write_registers(cpu, &cpu->regs);
}
