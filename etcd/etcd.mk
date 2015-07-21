.PHONY: all

VER := v2.1.1
ARCH := amd64
TARGET := etcd-$(VER)-linux-$(ARCH).aci

all: $(BUILDDIR)/$(TARGET) etcd.mk
	cp -af ./etcd.service $(BUILDDIR)/rootfs/lib/systemd/system
	mkdir -p $(BUILDDIR)/rootfs/opt/packages
	mkdir -p $(BUILDDIR)/rootfs/var/etcd
	cp -af $(BUILDDIR)/$(TARGET) $(BUILDDIR)/rootfs/opt/packages/etcd.aci
	ln -sf /lib/systemd/system/etcd.service  $(BUILDDIR)/rootfs/lib/systemd/system/multi-user.target.wants/

$(BUILDDIR)/$(TARGET):
	curl -L https://github.com/coreos/etcd/releases/download/$(VER)/$(TARGET) -o $(BUILDDIR)/$(TARGET)
