#ifndef __QCSCHED_CONSTANT_H
#define __QCSCHED_CONSTANT_H

#define MAX_SCHEDPOINTS 128
// TODO: Do not use this macro
#define MAX_CPUS 8

#define QCSCHED_DUMMY_BREAKPOINT ~(target_ulong)(0)

#define WATCHDOG_BREAKPOINT_COUNT_MAX 10
#define WATCHDOG_BREAKPOINT_COUNT_KILL_QEMU 20

#define END_OF_SCHEDPOINT_WINDOW MAX_SCHEDPOINTS + 1

#define TRAMPOLINE_ESCAPE_MAGIC 0x75da1791

enum qcschedpoint_footprint {
    // Not yet handled
    footprint_preserved = 0,
    // The schedpoint was missed. Should be removed from the scheudle
    footprint_missed,
    // The schedpoint was dropped. Should try again.
    footprint_dropped,
    // The schedpoint is hit.
    footprint_hit,
    // The schedpoint is not addressed
    footprint_not_addressed,
};

#endif /* __QCSCHED_CONSTANT_H */
