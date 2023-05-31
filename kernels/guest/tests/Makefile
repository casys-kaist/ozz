OUTDIR=build/
SRC=$(wildcard **/*.c)
DST=$(patsubst %.c,%,$(addprefix $(OUTDIR),$(SRC)))

all: prep $(DST)

prep:
	@mkdir -p $(OUTDIR)/concurrency_bugs
	@mkdir -p $(OUTDIR)/reordering_bugs
	@mkdir -p $(OUTDIR)/tests

build/tests/%: tests/%.c
	@gcc $< -o $@ -x86_64 -lpthread --static -I./include

build/concurrency_bugs/%: concurrency_bugs/%.c
	@gcc $< -o $@ -x86_64 -lpthread --static -I./include

build/reordering_bugs/%: reordering_bugs/%.c
	@gcc $< -o $@ -x86_64 -lpthread --static -lkeyutils -I./include

clean:
	@rm -rf $(OUTDIR)

.PHONY: all prep clean
