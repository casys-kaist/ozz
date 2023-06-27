#include "qemu/osdep.h"

#include <linux/kvm.h>

#include "sysemu/kvm.h"
#include "exec/gdbstub.h"

int kvm_insert_breakpoint_cpu(CPUState *cpu, target_ulong addr,
                                  target_ulong len, int type)
{
    int err = 0;
    if (type == GDB_BREAKPOINT_SW) {
        // TODO
    } else {
        err = kvm_arch_insert_hw_breakpoint_cpu(cpu, addr, len, type);
    }

    if (err)
        return err;

    return kvm_update_guest_debug(cpu, 0);
}

int kvm_remove_breakpoint_cpu(CPUState *cpu, target_ulong addr,
                                  target_ulong len, int type)
{
    int err = 0;
    if (type == GDB_BREAKPOINT_SW) {
        // TODO
    } else {
        err = kvm_arch_remove_hw_breakpoint_cpu(cpu, addr, len, type);
    }

    if (err)
        return err;

    return kvm_update_guest_debug(cpu, 0);
}

void kvm_remove_all_breakpoints_cpu(CPUState *cpu)
{
    kvm_arch_remove_all_hw_breakpoints_cpu(cpu);
    kvm_update_guest_debug(cpu, 0);
}
