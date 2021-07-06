MODULES := $(wildcard */)
MODULES := $(filter-out include/, $(MODULES))
C_INCLUDE_PATH := $(shell pwd)/include:$(C_INCLUDE_PATH)
export C_INCLUDE_PATH

all: $(MODULES)
$(MODULES):
	(cd $@; $(MAKE))

UPLOAD_SCRIPT = $(SCRIPTS_DIR)/qemu/upload.sh
upload:
	find . -name "*.ko" | xargs $(UPLOAD_SCRIPT)


.PHONY: all $(MODULES) upload

