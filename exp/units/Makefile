OUTDIR=build/
SRC=$(wildcard *.c)
DST=$(patsubst %.c,%,$(addprefix $(OUTDIR),$(SRC)))

all: prep $(DST)

prep:
	@mkdir -p $(OUTDIR)

build/%: %.c test.h
	@gcc $< -o $@ -lpthread --static

clean:
	@rm -rf $(OUTDIR)

.PHONY: all prep clean
