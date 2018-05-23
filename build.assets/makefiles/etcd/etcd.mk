.PHONY: all

ARCH := amd64


all: $(ETCD_VER)
	@echo -e "\n---> Building etcd:\n"

	@echo -e "\n---> Setup etcd services:\n"
	cd $(ASSETDIR)
	cp -afv ./etcd.service $(ROOTFS)/lib/systemd/system/
	cp -afv ./etcd-upgrade.service $(ROOTFS)/lib/systemd/system/
	cp -afv ./etcd-gateway.dropin $(ROOTFS)/lib/systemd/system/
	cp -afv ./etcdctl3 $(ROOTFS)/usr/bin/etcdctl3
	chmod +x $(ROOTFS)/usr/bin/etcdctl3
	ln -sf /lib/systemd/system/etcd.service $(ROOTFS)/lib/systemd/system/multi-user.target.wants/

	# mask the etcd-upgrade service so that it can only be run if intentionally unmasked
	ln -sf /dev/null $(ROOTFS)/etc/systemd/system/etcd-upgrade.service

	# Write to the release file to indicate the latest release
	echo PLANET_ETCD_VERSION=$(ETCD_LATEST_VER) >> $(ROOTFS)/etc/planet-release

.PHONY: $(ETCD_VER)
$(ETCD_VER):
	@echo -e "\n---> $@ - Downloading etcd\n"
	curl -L https://github.com/coreos/etcd/releases/download/$@/etcd-$@-linux-$(ARCH).tar.gz \
	-o $(ASSETDIR)/$@.tar.gz;

	@echo -e "\n---> $@ - Extracting etcd\n"
	cd $(ASSETDIR)
	tar -xzf $(ASSETDIR)/$@.tar.gz

	cp -afv etcd-$@-linux-$(ARCH)/etcd $(ROOTFS)/usr/bin/etcd-$@
	cp -afv etcd-$@-linux-$(ARCH)/etcdctl $(ROOTFS)/usr/bin/etcdctl-$@
