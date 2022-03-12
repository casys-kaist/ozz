#ifndef __QCSCHED_CONSTANT_H
#define __QCSCHED_CONSTANT_H

#define MAX_SCHEDPOINTS 128
// TODO: Do not use this macro
#define MAX_CPUS 8

#define QCSCHED_DUMMY_BREAKPOINT ~(target_ulong)(0)

#define WATCHDOG_BREAKPOINT_COUNT_MAX 10

#define TRAMPOLINE_ESCAPE_MAGIC 0x75da1791

#endif /* __QCSCHED_CONSTANT_H */