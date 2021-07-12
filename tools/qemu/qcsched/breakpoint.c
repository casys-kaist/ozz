#define _DEBUG

#include "qemu/osdep.h"

#include "exec/gdbstub.h"
#include "qemu/main-loop.h"
#include "sysemu/kvm.h"

#include "qemu/qcsched/qcsched.h"
#include "qemu/qcsched/vmi.h"

#define RIP(cpu) (cpu->regs.rip)

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

static void kidnap_task(CPUState *cpu)
{
    struct qcsched_trampoline_info *trampoline = get_trampoline_info(cpu);

    if (sched.current == sched.total)
        // We hit the last breakpoint. TODO: This if statement allows
        // thread execute parallel after the last breakpoint. We may
        // want to a better scheduling mechanism.
        return;

    // TODO: Do we want to kidnap more than one thread?
    ASSERT(!trampoline->trampoled, "kidnapping more than one thread");

    DRPRINTF(cpu, "kidnapping\n");
    __copy_registers(&trampoline->orig_regs, &cpu->regs);
    jump_into_trampoline(cpu);
    trampoline->trampoled = true;
}

static void resume_task(CPUState *cpu)
{
    struct qcsched_trampoline_info *trampoline = get_trampoline_info(cpu);

    ASSERT(trampoline->trampoled, "nothing has been kidnapped");

    DRPRINTF(cpu, "resumming\n");
    __copy_registers(&cpu->regs, &trampoline->orig_regs);
    cpu->qcsched_dirty = true;
    memset(trampoline, 0, sizeof(*trampoline));
}

static void hand_over_baton(CPUState *cpu)
{
    sched.current = sched.current + 1;
    DRPRINTF(cpu, "Next scheduling point: %d, %lx\n", sched.current,
             sched.entries[sched.current].schedpoint.addr);
}

static void wake_cpu_up(CPUState *cpu, CPUState *wakeup)
{
    // Installing a breakpoint on the trampoline so each CPU can
    // wake up on its own.
    DRPRINTF(cpu, "waking cpu #%d\n", wakeup->cpu_index);
    ASSERT(!kvm_insert_breakpoint_cpu(wakeup, vmi_info.trampoline_exit_addr, 1,
                                      GDB_BREAKPOINT_HW),
           "failing to wake cpu #%d up", wakeup->cpu_index);
}

static void wake_others_up(CPUState *cpu0)
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

static bool breakpoint_on_hook(CPUState *cpu)
{
    return RIP(cpu) == vmi_info.hook_addr;
}

static bool breakpoint_on_trampoline(CPUState *cpu)
{
    return RIP(cpu) == vmi_info.trampoline_entry_addr ||
           RIP(cpu) == vmi_info.trampoline_exit_addr;
}

static bool breakpoint_on_schedpoint(CPUState *cpu)
{
    struct qcsched_entry *entry;
    struct qcsched_vmi_task running;
    int i;

    qcsched_vmi_task(cpu, &running);

    for (i = 0; i < sched.total; i++) {
        entry = &sched.entries[i];
        if (entry->schedpoint.addr == RIP(cpu) &&
            vmi_same_task(&running, &entry->t))
            return true;
    }
    return false;
}

static void __handle_breakpoint_hook(CPUState *cpu)
{
    DRPRINTF(cpu, "%s\n", __func__);
    // If the task can make a progress, we don't need to do something.
    if (!qcsched_vmi_can_progress(cpu))
        kidnap_task(cpu);
}

static void __handle_breakpoint_trampoline(CPUState *cpu)
{
    DRPRINTF(cpu, "%s\n", __func__);
    // Each cpu determines that it can make a progress.
    if (qcsched_vmi_can_progress(cpu))
        resume_task(cpu);
}

static void __handle_breakpoint_schedpoint(CPUState *cpu)
{
    DRPRINTF(cpu, "%s (%llx)\n", __func__, RIP(cpu));
    // Hand over the baton to the next task first
    hand_over_baton(cpu);
    // and then kidnap the executing task
    kidnap_task(cpu);
    // And then wake others up
    wake_others_up(cpu);
}

int qcsched_handle_breakpoint(CPUState *cpu)
{
    // Remove the breakpoint first
    ASSERT(!kvm_remove_breakpoint_cpu(cpu, RIP(cpu), 1, GDB_BREAKPOINT_HW),
           "failed to remove breakpoint\n");

    qemu_mutex_lock_iothread();
    if (breakpoint_on_hook(cpu)) {
        __handle_breakpoint_hook(cpu);
    } else if (breakpoint_on_trampoline(cpu)) {
        __handle_breakpoint_trampoline(cpu);
    } else if (breakpoint_on_schedpoint(cpu)) {
        __handle_breakpoint_schedpoint(cpu);
    } else {
        // Unknown case. Might be an error.
        DRPRINTF(cpu, "Unknown breakpoint: %llx\n", RIP(cpu));
    }
    qemu_mutex_unlock_iothread();

    return 0;
}

void qcsched_escape_if_trampoled(CPUState *cpu, CPUState *wakeup)
{
    struct qcsched_trampoline_info *trampoline = get_trampoline_info(wakeup);
    if (trampoline->trampoled)
        wake_cpu_up(cpu, wakeup);
}
