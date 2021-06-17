OUTDIR=build/
SRC=$(wildcard *.c)
DEPS=$(patsubst %.c, $(OUTDIR)/%, $(SRC))

all: $(DEPS)

prep:
	mkdir -p $(OUTDIR)

$(OUTDIR)/%: %.c
	@make prep
	@gcc $< -o $@ -x86_64 -lpthread
	@aarch64-linux-gnu-gcc $< -o $@-aarch64 -lpthread

clean:
	@rm -rf $(OUTDIR)

.PHONY: all prep clean
