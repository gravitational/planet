.PHONY: all

VER := v2.1.1
ARCH := amd64
TARGET := etcd-$(VER)-linux-$(ARCH)
TARGET_TARBALL := $(TARGET).tar.gz

all: $(BUILDDIR)/$(TARGET) etcd.mk
	cp -af ./etcd.service $(ROOTFS)/lib/systemd/system
	mkdir -p $(ROOTFS)/var/etcd
	cp -af $(BUILDDIR)/$(TARGET)/etcd $(ROOTFS)/usr/bin
	cp -af $(BUILDDIR)/$(TARGET)/etcdctl $(ROOTFS)/usr/bin
	ln -sf /lib/systemd/system/etcd.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
	install -m 0755 ./wait-for-etcd.sh $(ROOTFS)/usr/bin

$(BUILDDIR)/$(TARGET):
	curl -L https://github.com/coreos/etcd/releases/download/$(VER)/$(TARGET_TARBALL) -o $(BUILDDIR)/$(TARGET_TARBALL)
	cd $(BUILDDIR) && tar -xzf $(BUILDDIR)/$(TARGET_TARBALL)
