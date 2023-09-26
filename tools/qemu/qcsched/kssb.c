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

static bool is_vcpu_deactivated(CPUState *cpu)
{
    return !qcsched_check_cpu_state(cpu, qcsched_cpu_ready) ||
           qcsched_check_cpu_state(cpu, qcsched_cpu_deactivated);
}

static bool is_all_vcpus_deactivated(CPUState *cpu)
{
    CPUState *cpu0;

    CPU_FOREACH(cpu0)
    {
        if (!is_vcpu_deactivated(cpu0)) {
            DRPRINTF(cpu, "CPU #%d is not deactivated (state: %d)\n",
                     cpu0->cpu_index, sched.cpu_state[cpu0->cpu_index]);
            return false;
        }
    }
    return true;
}

target_ulong qcsched_enable_kssb(CPUState *cpu)
{
    bool ok = true;
    DRPRINTF(cpu, "Enabling kssb\n");
    if (!sched.kssb_enabled) {
        ok = qcsched_rw__ssb_do_emulate(cpu, 1);
        sched.kssb_enabled = ok;
    } else {
        DRPRINTF(cpu, "Kssb is already enabled\n");
    }
    return (ok ? 0 : -EINVAL);
}

target_ulong qcsched_disable_kssb(CPUState *cpu)
{
    bool ok;

    DRPRINTF(cpu, "Disabling kssb\n");
    if (!sched.kssb_enabled) {
        DRPRINTF(cpu, "Kssb is already disabled\n");
        return 0;
    }

    if (!is_all_vcpus_deactivated(cpu)) {
        DRPRINTF(cpu, "Failed to disable kssb\n");
        return -EAGAIN;
    }

    vm_stop(RUN_STATE_PAUSED);
    ok = qcsched_rw__ssb_do_emulate(cpu, 0);
    // TODO: I don't think this is a correct way to use
    // vm_prepare_start() and resume_all_vcpus(). It works for now,
    // but it would be better to fix it later.
    vm_prepare_start();
    resume_all_vcpus();
    sched.kssb_enabled = !ok;
    return (ok ? 0 : -EINVAL);
}
