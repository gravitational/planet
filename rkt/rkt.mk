.PHONY: all patch-stage1

VER := v0.7.0
DISTRO := rkt-$(VER).tar.gz

all: $(BUILDDIR)/$(DISTRO) rkt.mk
	cd $(BUILDDIR) && tar -xzf $(DISTRO)

# patch stage1 to add missing libraries
	rm -rf $(BUILDDIR)/stage1-patch
	mkdir -p $(BUILDDIR)/stage1-patch
	cp $(BUILDDIR)/rkt-$(VER)/stage1.aci $(BUILDDIR)/stage1-patch
	cd $(BUILDDIR)/stage1-patch && tar -xzf stage1.aci && rm -f stage1.aci

	mkdir -p $(BUILDDIR)/stage1-patch/rootfs/lib
	cp -af /lib/libip4tc.so.0 $(BUILDDIR)/stage1-patch/rootfs/lib/libip4tc.so.0

	mkdir -p $(BUILDDIR)/stage1-patch/rootfs/etc
	cp -af ./rootfs/etc/resolv.conf $(BUILDDIR)/stage1-patch/rootfs/etc

	cd $(BUILDDIR) && actool build -overwrite stage1-patch stage1.aci
	mkdir -p $(ROOTFS)/usr/bin/rkt
	cp -af $(BUILDDIR)/rkt-$(VER)/rkt $(ROOTFS)/usr/bin/rkt/
	cp -af $(BUILDDIR)/stage1.aci $(ROOTFS)/usr/bin/rkt/

# install socket-activated metadata service
	cp -af ./rkt-metadata.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/rkt-metadata.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/

	cp -af ./rkt-metadata.socket $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/rkt-metadata.socket  $(ROOTFS)/lib/systemd/system/sockets.target.wants/


$(BUILDDIR)/$(DISTRO):
	curl -L https://github.com/coreos/rkt/releases/download/$(VER)/rkt-$(VER).tar.gz -o $(BUILDDIR)/$(DISTRO)
