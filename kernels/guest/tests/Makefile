OUTDIR=build/
SRC=$(wildcard **/*.c)
DST=$(patsubst %.c,%,$(addprefix $(OUTDIR),$(SRC)))

all: prep $(DST)

prep:
	@mkdir -p $(OUTDIR)/tests

print_notice:
    $(info ********************************************************)
    $(info **  NOTICE NOTICE NOTICE NOTICE NOTICE NOTICE NOTICE  **)
    $(info **                                                    **)
    $(info **    In tests programs, addresses where breakpoints  **)
    $(info **    should be installed are hard-coded. Please      **)
    $(info **    make sure those addresses are correct before    **)
    $(info **    running test programs.                          **)
    $(info **                                                    **)
    $(info **  NOTICE NOTICE NOTICE NOTICE NOTICE NOTICE NOTICE  **)
    $(info ********************************************************)

build/tests/%: tests/%.c
	@gcc $< -o $@ -x86_64 -lpthread --static -I./include

clean:
	@rm -rf $(OUTDIR)

.PHONY: all prep print_notice clean
