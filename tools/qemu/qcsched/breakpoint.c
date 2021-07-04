#define _DEBUG

#include "qemu/osdep.h"

#include "sysemu/kvm.h"
#include "exec/gdbstub.h"

#include "qemu/qcsched/qcsched.h"

int qcsched_handle_breakpoint(CPUState *cpu)
{
    DRPRINTF("%s\n", __func__);
    kvm_remove_all_breakpoints_cpu(cpu);
    return 0;
}
