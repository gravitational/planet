.PHONY: all

VER := v2.1.1
ARCH := amd64
TARGET := etcd-$(VER)-linux-$(ARCH)
TARGET_TARBALL := $(TARGET).tar.gz

DOWNLOAD:=$(TARGETDIR)/$(TARGET_TARBALL)

all: $(DOWNLOAD)
	cd $(TARGETDIR) && tar -xzf $(TARGETDIR)/$(TARGET_TARBALL)
	mkdir -p $(ROOTFS)/var/etcd
	cp -afv $(TARGETDIR)/$(TARGET)/etcd $(ROOTFS)/usr/bin
	cp -afv $(TARGETDIR)/$(TARGET)/etcdctl $(ROOTFS)/usr/bin
	cp -afv ./etcd.service $(ROOTFS)/lib/systemd/system/
	ln -sf /lib/systemd/system/etcd.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/

$(DOWNLOAD):
	@echo "\n---> Building etcd \n"
	curl -L https://github.com/coreos/etcd/releases/download/$(VER)/$(TARGET_TARBALL) -o $(DOWNLOAD)
