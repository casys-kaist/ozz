#ifndef __HCALL_H
#define __HCALL_H

#ifdef CONFIG_QCSCHED

#include "hcall_constant.h"

bool qcsched_enable_kssb(CPUState *cpu);
bool qcsched_disable_kssb(CPUState *cpu);

#endif /* CONFIG_QCSCHED */

#endif /* __HCALL_H */
