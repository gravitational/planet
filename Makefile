.PHONY: all image

BUILDDIR := $(abspath build)
export

all:
	mkdir -p $(BUILDDIR)
	$(MAKE) -C image -f image.mk
	$(MAKE) -C rkt -f rkt.mk
