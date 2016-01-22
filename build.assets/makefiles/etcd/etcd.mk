.PHONY: all

VER := v2.2.4
ARCH := amd64
TARGET := etcd-$(VER)-linux-$(ARCH)
TARGET_TARBALL := $(TARGET).tar.gz

DOWNLOAD:=$(ASSETDIR)/$(TARGET_TARBALL)

all: $(DOWNLOAD)
	@echo "\n---> Building etcd:\n"
	cd $(ASSETDIR) && tar -xzf $(ASSETDIR)/$(TARGET_TARBALL)
	mkdir -p $(ROOTFS)/var/etcd
	cp -afv $(ASSETDIR)/$(TARGET)/etcd $(ROOTFS)/usr/bin
	cp -afv $(ASSETDIR)/$(TARGET)/etcdctl $(ROOTFS)/usr/bin
	cp -afv ./etcd.service $(ROOTFS)/lib/systemd/system/
	ln -sf /lib/systemd/system/etcd.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/

$(DOWNLOAD):
	curl -L https://github.com/coreos/etcd/releases/download/$(VER)/$(TARGET_TARBALL) -o $(DOWNLOAD)
