.PHONY: all

VER := v2.1.1
ARCH := amd64
TARGET := etcd-$(VER)-linux-$(ARCH)
TARGET_TARBALL := $(TARGET).tar.gz

all: $(OUT)/$(TARGET) etcd.mk
	cp -af ./etcd.service $(ROOTFS)/lib/systemd/system
	mkdir -p $(ROOTFS)/var/etcd
	cp -af $(OUT)/$(TARGET)/etcd $(ROOTFS)/usr/bin
	cp -af $(OUT)/$(TARGET)/etcdctl $(ROOTFS)/usr/bin
	ln -sf /lib/systemd/system/etcd.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/

$(OUT)/$(TARGET):
	curl -L https://github.com/coreos/etcd/releases/download/$(VER)/$(TARGET_TARBALL) -o $(OUT)/$(TARGET_TARBALL)
	cd $(OUT) && tar -xzf $(OUT)/$(TARGET_TARBALL)
