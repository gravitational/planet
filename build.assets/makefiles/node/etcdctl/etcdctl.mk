.PHONY: all

VER := v2.1.1
ARCH := amd64
TARGET := etcd-$(VER)-linux-$(ARCH)
TARGET_TARBALL := $(TARGET).tar.gz

all: $(TARGETDIR)/$(TARGET) etcdctl.mk
	@echo "\n---> Building etcdctl\n"
	cp -af $(TARGETDIR)/$(TARGET)/etcdctl $(ROOTFS)/usr/bin

$(TARGETDIR)/$(TARGET):
	curl -L https://github.com/coreos/etcd/releases/download/$(VER)/$(TARGET_TARBALL) -o $(TARGETDIR)/$(TARGET_TARBALL)
	cd $(TARGETDIR) && tar -xzf $(TARGETDIR)/$(TARGET_TARBALL)
