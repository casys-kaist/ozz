#ifndef __QCSCHED_CONSTANT_H
#define __QCSCHED_CONSTANT_H

#define MAX_SCHEDPOINTS 128
// TODO: Do not use this macro
#define MAX_CPUS 8

#define QCSCHED_DUMMY_BREAKPOINT ~(target_ulong)(0)

#define WATCHDOG_BREAKPOINT_COUNT_MAX 10

#define END_OF_SCHEDPOINT_WINDOW MAX_SCHEDPOINTS + 1

#define TRAMPOLINE_ESCAPE_MAGIC 0x75da1791

enum qcschedpoint_footprint {
    footprint_preserved = 0,
    footprint_missed,
    footprint_hit,
    footprint_dropped,
};

#endif /* __QCSCHED_CONSTANT_H */
