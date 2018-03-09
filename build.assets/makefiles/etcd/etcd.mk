.PHONY: all

ARCH := amd64
TARGET := etcd-$(ETCD_VER)-linux-$(ARCH)
TARGET_TARBALL := $(TARGET).tar.gz
DOWNLOAD:=$(ASSETDIR)/$(TARGET_TARBALL)

# Until we no longer support upgrades from etcd2, we need to install both etcd3 and etcd2
TARGET3 := etcd-$(ETCD3_VER)-linux-$(ARCH)
TARGET3_TARBALL := $(TARGET3).tar.gz
DOWNLOAD3:=$(ASSETDIR)/$(TARGET3_TARBALL)

all: $(DOWNLOAD)
	@echo "\n---> Building etcd:\n"
	cd $(ASSETDIR) && mkdir -p $(TARGET) && tar -xzf $(ASSETDIR)/$(TARGET_TARBALL) -C $(TARGET)
	mkdir -p $(ROOTFS)/var/etcd
	cp -afv $(ASSETDIR)/$(TARGET)/$(TARGET)/etcd $(ROOTFS)/usr/bin/etcd-$(ETCD_VER)
	cp -afv $(ASSETDIR)/$(TARGET)/$(TARGET)/etcdctl $(ROOTFS)/usr/bin/etcdctl-$(ETCD_VER)
	cp -afv ./etcd.service $(ROOTFS)/lib/systemd/system/
	cp -afv ./etcd-upgrade.service $(ROOTFS)/lib/systemd/system/
	ln -sf /lib/systemd/system/etcd.service $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
	# mask the etcd upgrade service so that it can only be run if intentionally unmasked
	ln -sf /dev/null $(ROOTFS)/etc/systemd/system/etcd-upgrade.service

	# ETCD3
	cd $(ASSETDIR) && mkdir -p $(TARGET3) && tar -xzf $(ASSETDIR)/$(TARGET3_TARBALL) -C $(TARGET3)
	cp -afv $(ASSETDIR)/$(TARGET3)/$(TARGET3)/etcd $(ROOTFS)/usr/bin/etcd-$(ETCD3_VER)
	cp -afv $(ASSETDIR)/$(TARGET3)/$(TARGET3)/etcdctl $(ROOTFS)/usr/bin/etcdctl-$(ETCD3_VER)

	# Default to newest supported etcd
	cd $(ROOTFS)/usr/bin/ && ln -sf etcd-$(ETCD3_VER) etcd
	cd $(ROOTFS)/usr/bin/ && ln -sf etcdctl-$(ETCD3_VER) etcdctl

	# Write to the release file to indicate the latest release
	echo ETCD_VERSION=$(ETCD3_VER) >> $(ROOTFS)/etc/planet-release

$(DOWNLOAD):
	curl -L https://github.com/coreos/etcd/releases/download/$(ETCD_VER)/$(TARGET_TARBALL) -o $(DOWNLOAD); \
	curl -L https://github.com/coreos/etcd/releases/download/$(ETCD3_VER)/$(TARGET3_TARBALL) -o $(DOWNLOAD3); \
