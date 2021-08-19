#ifndef __HCALL_H
#define __HCALL_H

#ifdef CONFIG_QCSCHED

#include "hcall_constant.h"

target_ulong qcsched_enable_kssb(CPUState *cpu);
target_ulong qcsched_disable_kssb(CPUState *cpu);

#endif /* CONFIG_QCSCHED */

#endif /* __HCALL_H */
