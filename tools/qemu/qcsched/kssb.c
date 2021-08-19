#define _DEBUG

#include "qemu/osdep.h"

#include <linux/kvm.h>

#include "cpu.h"
#include "exec/gdbstub.h"
#include "qemu-common.h"
#include "qemu/main-loop.h"
#include "sysemu/cpus.h"
#include "sysemu/runstate.h"

#include "qemu/qcsched/hcall.h"
#include "qemu/qcsched/qcsched.h"
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

target_ulong qcsched_enable_kssb(CPUState *cpu)
{
    bool ok;
    DRPRINTF(cpu, "Enabling kssb\n");
    ok = qcsched_rw__ssb_do_emulate(cpu, 1);
    return (ok ? 0 : -EINVAL);
}

target_ulong qcsched_disable_kssb(CPUState *cpu)
{
    bool ok;
    DRPRINTF(cpu, "Disabling kssb\n");
    vm_stop(RUN_STATE_PAUSED);
    ok = qcsched_rw__ssb_do_emulate(cpu, 0);
    // TODO: I don't think this is a correct way to use
    // vm_prepare_start() and resume_all_vcpus(). It works for now,
    // but it would be better to fix it later.
    vm_prepare_start();
    resume_all_vcpus();
    return (ok ? 0 : -EINVAL);
}
