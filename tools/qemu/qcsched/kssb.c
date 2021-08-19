#include "qemu/osdep.h"

#include <linux/kvm.h>

#include "cpu.h"
#include "exec/gdbstub.h"
#include "qemu-common.h"

#include "qemu/qcsched/qcsched.h"
#include "qemu/qcsched/hcall.h"
#include "qemu/qcsched/vmi.h"

static bool qcsched_rw__ssb_do_emulate(CPUState *cpu, char enable)
{
    target_ulong __ssb_do_emulate = vmi_info.__ssb_do_emulate;

    if (__ssb_do_emulate == 0)
        return false;

    ASSERT(!cpu_memory_rw_debug(cpu, __ssb_do_emulate, &enable, 1, 1),
           "Can't read __ssb_do_emulate");

    return true;
}

bool qcsched_enable_kssb(CPUState *cpu)
{
    bool ok = qcsched_rw__ssb_do_emulate(cpu, 1);
    return ok;
}

bool qcsched_disable_kssb(CPUState *cpu)
{
    bool ok = qcsched_rw__ssb_do_emulate(cpu, 0);
    return ok;
}
