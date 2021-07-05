MODULES := $(wildcard */)
MODULES := $(filter-out include/, $(MODULES))
C_INCLUDE_PATH := $(shell pwd)/include:$(C_INCLUDE_PATH)
export C_INCLUDE_PATH

all: $(MODULES)
$(MODULES):
	(cd $@; $(MAKE))

.PHONY: all $(MODULES)

