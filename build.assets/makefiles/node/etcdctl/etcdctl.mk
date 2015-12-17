.PHONY: all

VER := v2.1.1
ARCH := amd64
TARGET := etcd-$(VER)-linux-$(ARCH)
TARGET_TARBALL := $(TARGET).tar.gz

all: $(ASSETDIR)/$(TARGET) etcdctl.mk
	@echo "\n---> Building etcdctl:\n"
	cp -af $(ASSETDIR)/$(TARGET)/etcdctl $(ROOTFS)/usr/bin

$(ASSETDIR)/$(TARGET):
	curl -L https://github.com/coreos/etcd/releases/download/$(VER)/$(TARGET_TARBALL) -o $(ASSETDIR)/$(TARGET_TARBALL)
	cd $(ASSETDIR) && tar -xzf $(ASSETDIR)/$(TARGET_TARBALL)
