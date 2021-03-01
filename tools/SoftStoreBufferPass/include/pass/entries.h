#ifndef __IRQENTRIES_H
#define __IRQENTRIES_H

// In some architectures (e.g., x86_64), we can hardly identify a
// function is an irq entry with the given function definition. Let's
// use the extracted list of irq entries so we can easily identify
// them.
std::string IRQEntriesX86_64[] = {
#include "irqentries_x86_64.inc"
};

std::string SyscallEntryX86_64 = "do_syscall_64";

// TODO: I don't know all entries yet so I manually write down some
//  entries. It may be okay with these for our purpose. If it is not,
//  identify all entries and fill IRQEntriesArm64 with something
//  generated automatically.
std::string IRQEntriesArm64[] = {
    "handle_IRQ",         "do_page_fault", "do_translation_fault",
    "do_alignment_fault", "do_bad",
};

std::string SyscallEntryArm64 = "invoke_syscall";

#endif // __IRQENTRIES_H
