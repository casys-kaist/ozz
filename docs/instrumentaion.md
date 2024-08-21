# Instrumentation

We instrument a kernel to emulate the store buffer in a fully software
way.

## Where do we instrument

We instrument a kernel at various locations as specified in the LKMM
document as follows:

- load/store instructions
- atomic operations
- RCU APIs

Besides that we instrument at the entry and the exit basic block of
various handlers to make sure that the store buffer is flushed when
the context switch occurs.

- IRQ handler
- SoftIRQ handler
- Syscall handler

## How do we instrument

For most of instrumetation, we implement a LLVM pass
(`tools/SoftStoreBufferPass`) that instrument during compile
time. `scripts/linux/select_arch.sh` should be sourced before compling
the kernel to set proper environment variables (i.e., `CFLAGS_KSSB`)
that are used by the Kbuild system. The Kbuild system will provides
the environment variables to each compile unit depending on
`KSSB_INSTRUMENT` and `KSSB_INSTRUMENT_target.o`.

For atomic operations, we modify the kernel, as KASAN does, such that
atomic* call the flush callback before performing atomic operations
(see `f355631dea20e0b50f94091aeaa906e079d33e5f` of the Linux).

