.PHONY: all

VER := v2.1.1
ARCH := amd64
TARGET := etcd-$(VER)-linux-$(ARCH)
TARGET_TARBALL := $(TARGET).tar.gz

all: $(BUILDDIR)/$(TARGET) etcd.mk
	cp -af ./etcd.service $(BUILDDIR)/rootfs/lib/systemd/system
	mkdir -p $(BUILDDIR)/rootfs/opt/packages
	mkdir -p $(BUILDDIR)/rootfs/var/etcd
	cp -af $(BUILDDIR)/$(TARGET)/etcd $(BUILDDIR)/rootfs/usr/bin
	cp -af $(BUILDDIR)/$(TARGET)/etcdctl $(BUILDDIR)/rootfs/usr/bin
	ln -sf /lib/systemd/system/etcd.service  $(BUILDDIR)/rootfs/lib/systemd/system/multi-user.target.wants/

$(BUILDDIR)/$(TARGET):
	curl -L https://github.com/coreos/etcd/releases/download/$(VER)/$(TARGET_TARBALL) -o $(BUILDDIR)/$(TARGET_TARBALL)
	cd $(BUILDDIR) && tar -xzf $(BUILDDIR)/$(TARGET_TARBALL)
