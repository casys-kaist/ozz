#include "qemu/osdep.h"

#include "cpu.h"

#include "qemu/qcsched/qcsched.h"

void qcsched_eat_cookie(CPUState *cpu, enum qcsched_cookie type)
{
    switch (type) {
    case cookie_hcall:
        cpu->hcall_cookie++;
        break;
    case cookie_breakpoint:
        cpu->breakpoint_cookie++;
        break;
    case cookie_timer:
        cpu->timer_cookie++;
        break;
    default:
        break;
    }
}
