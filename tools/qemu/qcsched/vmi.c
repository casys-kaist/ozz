#define _DEBUG

#include "qemu/osdep.h"

#include "cpu.h"

#include "qemu/qcsched/hcall_constant.h"
#include "qemu/qcsched/qcsched.h"
#include "qemu/qcsched/vmi.h"
#include "qemu/qcsched/window.h"

struct qcsched_vmi_info vmi_info;

static void qcsched_vmi_hint_trampoline(CPUState *cpu, target_ulong addr,
                                        int index)
{
    DRPRINTF(cpu, "trampoline %s addr : %lx\n", (!index ? "entry" : "exit"),
             addr);
    vmi_info.trampoline_addr[index] = addr;
}

static void qcsched_vmi_hint_hook(CPUState *cpu, target_ulong addr)
{
    DRPRINTF(cpu, "hook addr: %lx\n", addr);
    vmi_info.hook_addr = addr;
}

static void qcsched_vmi_hint__per_cpu_offset(CPUState *cpu, int index,
                                             target_ulong addr)
{
    DRPRINTF(cpu, "__per_cpu_offset[%d]: %lx\n", index, addr);
    vmi_info.__per_cpu_offset[index] = addr;
}

static void qcsched_vmi_hint_current_task(CPUState *cpu, target_ulong addr)
{
    DRPRINTF(cpu, "current_task: %lx\n", addr);
    vmi_info.current_task = addr;
}

static void qcsched_vmi_hint__ssb_do_emulate(CPUState *cpu, target_ulong addr)
{
    DRPRINTF(cpu, "__ssb_do_dmulate: %lx\n", addr);
    vmi_info.__ssb_do_emulate = addr;
}

static void qcsched_vmi_hint__preempt_count(CPUState *cpu, target_ulong addr)
{
    DRPRINTF(cpu, "__preempt_count: %lx\n", addr);
    vmi_info.__preempt_count = addr;
}

static bool __vmi_scheduling_subject(struct qcsched_vmi_task *t)
{
    // We don't have that many entries. Just iterating is fast enough.
    int i;
    for (i = 0; i < sched.total; i++) {
        if (vmi_same_task(t, &sched.entries[i].t))
            return true;
    }
    return false;
}

static bool
qcsched_vmi_lock_info_duplicated(struct qcsched_vmi_lock_info *lock_info,
                                 struct qcsched_vmi_lock *vmi_lock)
{
    for (int i = 0; i < lock_info->count; i++) {
        if (memcmp(&lock_info->acquired[i], vmi_lock,
                   sizeof(struct qcsched_vmi_lock)) == 0)
            return true;
    }
    return false;
}

bool qcsched_vmi_running_context_being_scheduled(CPUState *cpu)
{
    struct qcsched_vmi_task running;
    qcsched_vmi_task(cpu, &running);
    return __vmi_scheduling_subject(&running) && qcsched_vmi_in_task(cpu);
}

static void qcsched_vmi_lock_acquire(CPUState *cpu, target_ulong lockdep_addr,
                                     int trylock, int read, target_ulong ip)
{
    struct qcsched_vmi_lock_info *lock_info =
        &vmi_info.lock_info[cpu->cpu_index];
    struct qcsched_schedpoint_window *window;
    int cnt = lock_info->count;
    struct qcsched_vmi_lock vmi_lock;

    if (!qcsched_vmi_running_context_being_scheduled(cpu))
        return;

    // Allowed: activated
    if (!qcsched_check_cpu_state(cpu, qcsched_cpu_activated) ||
        qcsched_check_cpu_state(cpu, qcsched_cpu_deactivated))
        return;

#ifdef _DEBUG_VERBOSE
    DRPRINTF(cpu, "lock_acquire, addr=%lx, trylock=%d, read=%d, ip=%lx\n",
             lockdep_addr, trylock, read, ip);
#endif

    // Can't hold more lock info
    if (cnt >= MAX_LOCKS)
        return;

    vmi_lock = (struct qcsched_vmi_lock){.lockdep_addr = lockdep_addr,
                                         .trylock = trylock,
                                         .read = read,
                                         .ip = ip};

    if (qcsched_vmi_lock_info_duplicated(lock_info, &vmi_lock))
        return;

    lock_info->acquired[cnt] = vmi_lock;
    lock_info->count = cnt + 1;

    if (qcsched_window_lock_contending(cpu)) {
        // This CPU is trying to acquire a lock and another CPU has
        // already acquired it. Let's yield a turn
        window = &sched.schedpoint_window[cpu->cpu_index];
        qcsched_yield_turn_from(cpu, window->from);
    }
}

static void qcsched_vmi_lock_release(CPUState *cpu, target_ulong lockdep_addr)
{
    struct qcsched_vmi_lock_info *lock_info =
        &vmi_info.lock_info[cpu->cpu_index];
    int cnt = lock_info->count;

    if (!qcsched_vmi_running_context_being_scheduled(cpu))
        return;

    // Allowed: activated
    if (!qcsched_check_cpu_state(cpu, qcsched_cpu_activated) ||
        qcsched_check_cpu_state(cpu, qcsched_cpu_deactivated))
        return;

#ifdef _DEBUG_VERBOSE
    DRPRINTF(cpu, "lock_release, addr=%lx\n", lockdep_addr);
#endif

    for (int i = 0; i < cnt; i++) {
        if (lockdep_addr == lock_info->acquired[i].lockdep_addr) {
            lock_info->acquired[i] = lock_info->acquired[cnt - 1];
            lock_info->count--;
            return;
        }
    }
}

target_ulong qcsched_vmi_hint(CPUState *cpu, target_ulong type,
                              target_ulong addr, target_ulong ip)
{
    int trylock, read;
    int index;
    switch (type) {
    case VMI_TRAMPOLINE ... VMI_TRAMPOLINE + 1:
        index = type - VMI_TRAMPOLINE;
        qcsched_vmi_hint_trampoline(cpu, addr, index);
        break;
    case VMI_HOOK:
        qcsched_vmi_hint_hook(cpu, addr);
        break;
    case VMI__PER_CPU_OFFSET0 ... VMI__PER_CPU_OFFSET0 + 63:
        index = type - VMI__PER_CPU_OFFSET0;
        qcsched_vmi_hint__per_cpu_offset(cpu, index, addr);
        break;
    case VMI_CURRENT_TASK:
        qcsched_vmi_hint_current_task(cpu, addr);
        break;
    case VMI__SSB_DO_EMULATE:
        qcsched_vmi_hint__ssb_do_emulate(cpu, addr);
        break;
    case VMI__PREEMPT_COUNT:
        qcsched_vmi_hint__preempt_count(cpu, addr);
        break;
    case VMI_LOCK_ACQUIRE:
        trylock = (addr >> 2) & 1;
        read = addr & 0x3;
        addr &= ~0x7;
        qcsched_vmi_lock_acquire(cpu, addr, trylock, read, ip);
        break;
    case VMI_LOCK_RELEASE:
        qcsched_vmi_lock_release(cpu, addr);
        break;
    default:
        DRPRINTF(cpu, "Unknown VMI type: %lx\n", type);
        return -EINVAL;
    }
    return 0;
}

void qcsched_vmi_lock_info_reset(CPUState *cpu)
{
    struct qcsched_vmi_lock_info *lock_info =
        &vmi_info.lock_info[cpu->cpu_index];
    lock_info->count = 0;
}

static unsigned int qcsched_vmi__preempt_count(CPUState *cpu)
{
    target_ulong __per_cpu_offset = vmi_info.__per_cpu_offset[cpu->cpu_index];
    uint8_t buf[32];
    target_ulong pcpu_ptr;
    unsigned int __preempt_count;

    if (__per_cpu_offset == 0)
        return 0;

    pcpu_ptr = __per_cpu_offset + vmi_info.__preempt_count;

    ASSERT(!cpu_memory_rw_debug(cpu, pcpu_ptr, buf, sizeof(int), 0),
           "Can't read pcpu section");

    __preempt_count = *(int *)buf;

    return __preempt_count;
}

bool qcsched_vmi_in_task(CPUState *cpu)
{
    target_ulong preempt_count = qcsched_vmi__preempt_count(cpu);
    bool in_nmi = preempt_count & NMI_MASK;
    bool in_hardirq = preempt_count & HARDIRQ_MASK;
    bool in_serving_softirq = (preempt_count & SOFTIRQ_MASK) & SOFTIRQ_OFFSET;
    return !(in_nmi | in_hardirq | in_serving_softirq);
}

static target_ulong current_task(CPUState *cpu)
{
    // TODO: This only works for x86_64
    uint8_t buf[128];
    target_ulong task, pcpu_ptr,
        __per_cpu_offset = vmi_info.__per_cpu_offset[cpu->cpu_index];

    if (__per_cpu_offset == 0)
        return 0;

    pcpu_ptr = __per_cpu_offset + vmi_info.current_task;

    ASSERT(!cpu_memory_rw_debug(cpu, pcpu_ptr, buf, TARGET_LONG_SIZE, 0),
           "Can't read pcpu section");

    task = *(target_ulong *)buf;
    return task;
}

void qcsched_vmi_task(CPUState *cpu, struct qcsched_vmi_task *t)
{
    // Use the current pointer in x86_64 until we have a better
    // option. It is stored in the per-cpu pointer called
    // current_task.
    t->task_struct = current_task(cpu);
}

bool vmi_same_task(struct qcsched_vmi_task *t0, struct qcsched_vmi_task *t1)
{
    return t0->task_struct == t1->task_struct;
}

bool qcsched_vmi_can_progress(CPUState *cpu)
{
    struct qcsched_entry *entry = &sched.entries[sched.current];
    struct qcsched_vmi_task running;
    qcsched_vmi_task(cpu, &running);
    // A running context can make a progress if
    //  1) it is a irrelevant context or
    //  2) the context is supposed to hit the next schedpoint or
    //  3) all schedpoint are handled or
    //  4) QCSCHED force the context to execute or
    //  5) the schedule is done
    return !__vmi_scheduling_subject(&running) ||
           vmi_same_task(&running, &entry->t) || sched.total == sched.current ||
           cpu->qcsched_force_wakeup || !sched.activated;
}

static bool lock_contending(struct qcsched_vmi_lock *l0,
                            struct qcsched_vmi_lock *l1)
{
    if (l0->lockdep_addr != l1->lockdep_addr)
        return false;

    if (l0->trylock || l1->trylock)
        return false;

    // TODO: How to handle read == 2?
    if (l0->read != 0 && l1->read != 0)
        return false;

    // (l0->lockdep_addr == l1->lockdep_addr) && (the same lock)
    // (l0->read == 0 || l1->read == 0)       && (at least one is exclusive)
    // (!l0->trylock && l1->trylock)          && (both need to acquire the
    // lock)
    return true;
}

static bool vmi_lock_info_contending(struct qcsched_vmi_lock_info *li0,
                                     struct qcsched_vmi_lock_info *li1)
{
    // NOTE: We could do this in O(n) using a hashmap, but we double
    // iterate over lock info because the number of locks at a given
    // time (= n) is not that large.
    int i, j;
    for (i = 0; i < li0->count; i++) {
        for (j = 0; j < li1->count; j++)
            if (lock_contending(&li0->acquired[i], &li1->acquired[j]))
                return true;
    }
    return false;
}

bool qcsched_vmi_lock_contending(CPUState *cpu0, CPUState *cpu1)
{
    struct qcsched_vmi_lock_info *li0, *li1;
    if (cpu0 == cpu1)
        return false;
    li0 = &vmi_info.lock_info[cpu0->cpu_index];
    li1 = &vmi_info.lock_info[cpu1->cpu_index];
    return vmi_lock_info_contending(li0, li1);
}
