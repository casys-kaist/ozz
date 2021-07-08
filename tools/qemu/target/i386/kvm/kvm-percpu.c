#include "qemu/osdep.h"

#include <linux/kvm.h>

#include "cpu.h"
#include "exec/gdbstub.h"
#include "kvm_i386.h"
#include "sysemu/kvm.h"
#include "sysemu/sysemu.h"

// NOTE: I don't embed hardware breakpoints in CPUState because cpu.h
// is used files that are not target-specific. This seems a bit dirty.
// Whatever.
struct percpu_hw_breakpoint {
    struct {
        target_ulong addr;
        int len;
        int type;
    } bp[4];
    int nb;
} hw_breakpoint[MAX_NR_CPUS];

static int find_hw_breakpoint_cpu(CPUState *cpu, target_ulong addr, int len,
                                  int type)
{
    int n;
    for (n = 0; n < hw_breakpoint[cpu->cpu_index].nb; n++) {
        if (hw_breakpoint[cpu->cpu_index].bp[n].addr == addr &&
            hw_breakpoint[cpu->cpu_index].bp[n].type == type &&
            (hw_breakpoint[cpu->cpu_index].bp[n].len == len || len == -1)) {
            return n;
        }
    }
    return -1;
}

int kvm_arch_insert_hw_breakpoint_cpu(CPUState *cpu, target_ulong addr,
                                      target_ulong len, int type)
{
    int nb;
    switch (type) {
    case GDB_BREAKPOINT_HW:
        len = 1;
        break;
    default:
        // TODO: All other breakpoint types are not yet implemented.
        return -ENOSYS;
    }

    if (hw_breakpoint[cpu->cpu_index].nb == 4) {
        return -ENOBUFS;
    }

    if (find_hw_breakpoint_cpu(cpu, addr, len, type) >= 0) {
        return -EEXIST;
    }

    nb = hw_breakpoint[cpu->cpu_index].nb;
    hw_breakpoint[cpu->cpu_index].bp[nb].addr = addr;
    hw_breakpoint[cpu->cpu_index].bp[nb].len = len;
    hw_breakpoint[cpu->cpu_index].bp[nb].type = type;
    hw_breakpoint[cpu->cpu_index].nb++;

    return 0;
}

int kvm_arch_remove_hw_breakpoint_cpu(CPUState *cpu, target_ulong addr,
                                      target_ulong len, int type)
{
    int nb, n;
    n = find_hw_breakpoint_cpu(cpu, addr, (type == GDB_BREAKPOINT_HW) ? 1 : len,
                               type);
    if (n < 0) {
        return -ENOENT;
    }
    nb = --hw_breakpoint[cpu->cpu_index].nb;
    hw_breakpoint[cpu->cpu_index].bp[n] = hw_breakpoint[cpu->cpu_index].bp[nb];

    return 0;
}

void kvm_arch_remove_all_hw_breakpoints_cpu(CPUState *cpu)
{
    hw_breakpoint[cpu->cpu_index].nb = 0;
}

void kvm_arch_update_guest_debug_cpu(CPUState *cpu, struct kvm_guest_debug *dbg)
{
    const uint8_t type_code[] = {[GDB_BREAKPOINT_HW] = 0x0,
                                 [GDB_WATCHPOINT_WRITE] = 0x1,
                                 [GDB_WATCHPOINT_ACCESS] = 0x3};
    const uint8_t len_code[] = {[1] = 0x0, [2] = 0x1, [4] = 0x3, [8] = 0x2};
    int n;

    if (hw_breakpoint[cpu->cpu_index].nb > 0) {
        dbg->control |= KVM_GUESTDBG_ENABLE | KVM_GUESTDBG_USE_HW_BP;
        dbg->arch.debugreg[7] = 0x0600;
        for (n = 0; n < hw_breakpoint[cpu->cpu_index].nb; n++) {
            dbg->arch.debugreg[n] = hw_breakpoint[cpu->cpu_index].bp[n].addr;
            dbg->arch.debugreg[7] |=
                (2 << (n * 2)) |
                (type_code[hw_breakpoint[cpu->cpu_index].bp[n].type]
                 << (16 + n * 4)) |
                ((uint32_t)len_code[hw_breakpoint[cpu->cpu_index].bp[n].len]
                 << (18 + n * 4));
        }
    }
}
