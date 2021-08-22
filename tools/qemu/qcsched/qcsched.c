#define _DEBUG

#include "qemu/osdep.h"

#include <linux/kvm.h>

#include "cpu.h"
#include "qemu/main-loop.h"
#include "sysemu/kvm.h"

#include "qemu/qcsched/qcsched.h"

#include <sys/syscall.h>
#include <sys/types.h>
#include <time.h>
#include <unistd.h>

struct qcsched sched;

#define INTERRUPT_FLAG_BIT 9
#define INTERRUPT_FLAG_MASK 0x0200

void qcsched_pre_run(CPUState *cpu)
{
    if (cpu->qcsched_disable_irq) {
        DRPRINTF(cpu, "Disabling irq\n");
        cpu->qcsched_orig_irq_enabled =
            !!(cpu->regs.rflags & INTERRUPT_FLAG_MASK);
        cpu->regs.rflags &= ~INTERRUPT_FLAG_MASK;
        cpu->qcsched_disable_irq = false;
        cpu->qcsched_dirty = true;
    }

    if (cpu->qcsched_restore_irq) {
        DRPRINTF(cpu, "Restoring irq\n");
        cpu->regs.rflags |=
            (cpu->qcsched_orig_irq_enabled << INTERRUPT_FLAG_BIT) &
            INTERRUPT_FLAG_MASK;
        cpu->qcsched_orig_irq_enabled = false;
        cpu->qcsched_restore_irq = false;
        cpu->qcsched_dirty = true;
    }

    if (cpu->qcsched_dirty) {
        ASSERT(!kvm_write_registers(cpu, &cpu->regs),
               "failed to write registers");
        cpu->qcsched_dirty = false;
    }
}

void qcsched_post_run(CPUState *cpu)
{
    ASSERT(!kvm_read_registers(cpu, &cpu->regs), "failed to read registers");
}

static void qcsched_skip_executed_vmcall(struct kvm_regs *regs)
{
#define VMCALL_INSN_LEN 3
    regs->rip += VMCALL_INSN_LEN;
}

void qcsched_commit_state(CPUState *cpu, target_ulong hcall_ret)
{
    qcsched_skip_executed_vmcall(&cpu->regs);
    cpu->regs.rax = hcall_ret;
    cpu->qcsched_dirty = true;
}

// NOTE: The man page for sigevent clearly specifies that struct
// sigevent has a member field 'sigev_notify_thread_id'. Indeed, the
// struct does not have the member field and, instead, it is defined
// as the macro below (see include/uapi/asm-generic/siginfo.h in the
// Linux repo). For some reasons, the macro in the header file does
// not work, so I copied it here as a workaround.
#define sigev_notify_thread_id _sigev_un._tid

#define gettid() syscall(SYS_gettid)

void qcsched_init_vcpu(CPUState *cpu)
{
    struct qcsched_trampoline_info *trampoline = get_trampoline_info(cpu);
    struct sigevent sevp;
    pid_t tid = gettid();
    sevp.sigev_notify = SIGEV_THREAD_ID;
    sevp.sigev_signo = SIG_IPI;
    sevp.sigev_value.sival_int = TRAMPOLINE_ESCAPE_MAGIC;
    sevp.sigev_notify_thread_id = tid;
    ASSERT(!timer_create(CLOCK_MONOTONIC, &sevp, &trampoline->timerid),
           "timer_create");
}
