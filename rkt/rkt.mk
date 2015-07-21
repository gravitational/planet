.PHONY: all

VER := v0.7.0
DISTRO := rkt-$(VER).tar.gz

all: $(BUILDDIR)/$(DISRO) rkt.mk
	cd $(BUILDDIR) && tar -xzf $(DISTRO)
	mkdir -p $(BUILDDIR)/rootfs/usr/bin/rkt
	cp -af $(BUILDDIR)/rkt-$(VER)/. $(BUILDDIR)/rootfs/usr/bin/rkt/

$(BUILDDIR)/$(DISTRO):
	curl -L https://github.com/coreos/rkt/releases/download/$(VER)/rkt-$(VER).tar.gz -o $(BUILDDIR)/$(DISTRO)
