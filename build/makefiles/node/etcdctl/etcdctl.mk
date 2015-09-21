.PHONY: all

VER := v2.1.1
ARCH := amd64
TARGET := etcd-$(VER)-linux-$(ARCH)
TARGET_TARBALL := $(TARGET).tar.gz

all: $(OUT)/$(TARGET) etcdctl.mk
	cp -af $(OUT)/$(TARGET)/etcdctl $(ROOTFS)/usr/bin

$(OUT)/$(TARGET):
	curl -L https://github.com/coreos/etcd/releases/download/$(VER)/$(TARGET_TARBALL) -o $(OUT)/$(TARGET_TARBALL)
	cd $(OUT) && tar -xzf $(OUT)/$(TARGET_TARBALL)
