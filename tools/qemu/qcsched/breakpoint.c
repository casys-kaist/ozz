#define _DEBUG

#include "qemu/osdep.h"

#include "exec/gdbstub.h"
#include "sysemu/kvm.h"

#include "qemu/qcsched/qcsched.h"

int qcsched_handle_breakpoint(CPUState *cpu)
{
    unsigned long rip = cpu->regs.rip;
    DRPRINTF("%s\n", __func__);
    kvm_remove_breakpoint_cpu(cpu, rip, 1, GDB_BREAKPOINT_HW);
    return 0;
}
