OUTDIR=build/
SRC=$(wildcard **/*.c)
DST=$(patsubst %.c,%,$(addprefix $(OUTDIR),$(SRC)))

all: prep $(DST)

prep:
	@mkdir -p $(OUTDIR)/poc
	@mkdir -p $(OUTDIR)/tests

build/tests/%: tests/%.c
	@gcc $< -o $@ -x86_64 -lpthread --static -I./include

build/poc/%: poc/%.c
	@gcc $< -o $@ -x86_64 -lpthread --static -lkeyutils -I./include

clean:
	@rm -rf $(OUTDIR)

.PHONY: all prep clean
