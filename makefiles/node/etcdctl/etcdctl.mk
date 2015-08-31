.PHONY: all

VER := v2.1.1
ARCH := amd64
TARGET := etcd-$(VER)-linux-$(ARCH)
TARGET_TARBALL := $(TARGET).tar.gz

all: $(BUILDDIR)/$(TARGET) etcdctl.mk
	cp -af $(BUILDDIR)/$(TARGET)/etcdctl $(ROOTFS)/usr/bin

$(BUILDDIR)/$(TARGET):
	curl -L https://github.com/coreos/etcd/releases/download/$(VER)/$(TARGET_TARBALL) -o $(BUILDDIR)/$(TARGET_TARBALL)
	cd $(BUILDDIR) && tar -xzf $(BUILDDIR)/$(TARGET_TARBALL)
