#!/bin/sh -e

# In some architectures (e.g., x86_64), we can hardly identify a
# function is an irq entry with the given function definition. Let's
# parse a source file and extract the list of irq entries so we can
# easily identify them.

LINUX="$PROJECT_HOME/kernels/linux"

extract_x86_64_idtentries() {
	X86_IDTENTRY_H="$LINUX/arch/x86/include/asm/idtentry.h"

	REGEXP="^DECLARE_IDTENTRY.*,[[:space:]]*([^\)]*).*\);"
	SEDCMD="s/$REGEXP/\"\1\",/p"

	DST_X86="$TOOLS_DIR/SoftStoreBufferPass/include/pass/irqentries_x86_64.inc"

	sed -nE "$SEDCMD" "$X86_IDTENTRY_H" > "$DST_X86"
}

extract_x86_64_idtentries
