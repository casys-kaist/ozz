#define _DEBUG

#include "qemu/osdep.h"

#include "exec/gdbstub.h"
#include "qemu/main-loop.h"
#include "sysemu/kvm.h"

#include "qemu/qcsched/qcsched.h"
#include "qemu/qcsched/trampoline.h"
#include "qemu/qcsched/vmi.h"
#include "qemu/qcsched/window.h"
