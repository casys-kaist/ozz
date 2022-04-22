#define _DEBUG

#include "qemu/osdep.h"

#include "exec/gdbstub.h"
#include "qemu/main-loop.h"
#include "sysemu/kvm.h"

#include "qemu/qcsched/qcsched.h"
#include "qemu/qcsched/trampoline.h"
#include "qemu/qcsched/vmi.h"
#include "qemu/qcsched/window.h"

// For the same reason for percpu_hw_breakpoint, I decide not to embed
// qcsched_trampoline_info in CPUState.
static struct qcsched_trampoline_info trampolines[MAX_NR_CPUS];

struct qcsched_trampoline_info *get_trampoline_info(CPUState *cpu)
{
    return &trampolines[cpu->cpu_index];
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

// XXX: Disabling and restoring IRQ somehow blocks a thread going back
// from the trampoline. RelRazzer requires this to hold the store
// buffer but RazzerV2 does not. During RazzerV2 I do not use this,
// but when doing RelRazzer, we need to inspect what is
// happening. Repeatedly running qcsched-test-simple/bypass will be
// helpful.
__attribute__((unused)) static void __disable_irq(CPUState *cpu)
{
    cpu->qcsched_disable_irq = true;
}

__attribute__((unused)) static void __restore_irq(CPUState *cpu)
{
    cpu->qcsched_restore_irq = true;
}

bool task_kidnapped(CPUState *cpu)
{
    struct qcsched_trampoline_info *trampoline = get_trampoline_info(cpu);
    return trampoline->trampoled;
}

void kidnap_task(CPUState *cpu)
{
    struct qcsched_trampoline_info *trampoline = get_trampoline_info(cpu);

    ASSERT(qcsched_vmi_running_context_being_scheduled(cpu),
           "kidnapping a wrong context");

    if (sched.current == sched.total || !sched.activated)
        // We hit the last breakpoint. TODO: This if statement allows
        // thread execute parallel after the last breakpoint. We may
        // want to a better scheduling mechanism.
        return;

    // TODO: Do we want to kidnap more than one thread?
    ASSERT(!trampoline->trampoled, "kidnapping more than one thread, cpu=%d",
           cpu->cpu_index);

    DRPRINTF(cpu, "kidnapping\n");
    __copy_registers(&trampoline->orig_regs, &cpu->regs);
    /* __disable_irq(cpu); */
    jump_into_trampoline(cpu);
    trampoline->trampoled = true;
    qcsched_arm_selfescape_timer(cpu);
}

void resume_task(CPUState *cpu)
{
    struct qcsched_trampoline_info *trampoline = get_trampoline_info(cpu);

    ASSERT(trampoline->trampoled, "nothing has been kidnapped");
    // These two asserts should be enforced to safely run with
    // qcsched_handle_kick().
    ASSERT(qemu_mutex_iothread_locked(), "iothread mutex is not locked");
    ASSERT(cpu == current_cpu, "something wrong: cpu != current_cpu");

    DRPRINTF(cpu, "resumming (force: %d)\n", cpu->qcsched_force_wakeup);
    __copy_registers(&cpu->regs, &trampoline->orig_regs);
    /* __restore_irq(cpu); */
    cpu->qcsched_dirty = true;
    cpu->qcsched_force_wakeup = false;
    memset(trampoline, 0, sizeof(*trampoline) - sizeof(timer_t));

    qcsched_window_expand_window(cpu);
}

void wake_cpu_up(CPUState *cpu, CPUState *wakeup)
{
    int r;
    // Installing a breakpoint on the trampoline so each CPU can
    // wake up on its own.
    DRPRINTF(cpu, "waking cpu #%d\n", wakeup->cpu_index);
    r = kvm_insert_breakpoint_cpu(wakeup, vmi_info.trampoline_exit_addr, 1,
                                  GDB_BREAKPOINT_HW);
    // The race condition scenario: one cpu is trying to wake another
    // cpu up, and the one is also trying to wake up on its own. It is
    // okay in this case because we install the breakpoint anyway. So
    // ignore -EEXIST.
    ASSERT(r == 0 || r == -EEXIST, "failing to wake cpu #%d up err=%d",
           wakeup->cpu_index, r);
}

void wake_others_up(CPUState *cpu0)
{
    CPUState *cpu;
    struct qcsched_trampoline_info *trampoline;

    CPU_FOREACH(cpu)
    {
        trampoline = get_trampoline_info(cpu);
        if (!trampoline->trampoled || cpu->cpu_index == cpu0->cpu_index)
            continue;
        wake_cpu_up(cpu0, cpu);
    }
}
