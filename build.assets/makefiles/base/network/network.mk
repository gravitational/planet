
CNI_VER := v0.4.0

CNI_TARBALL := cni-$(CNI_VER).tgz
CNI_TARGET := $(ASSETDIR)/$(CNI_TARBALL)

.PHONY: all
all: network.mk cni-install
	@echo "\\n---> Installing Flannel and preparing network stack for Kubernetes:\\n"
	mkdir -p $(ASSETDIR)
# script that allows waiting for etcd to come up
	mkdir -p $(ROOTFS)/usr/bin/scripts
	install -m 0755 ./wait-for-etcd.sh $(ROOTFS)/usr/bin/scripts
# script that sets up /etc/hosts and symlinks resolv.conf
	install -m 0755 ./setup-etc.sh $(ROOTFS)/usr/bin/scripts

.PHONY: cni-install
cni-install: $(CNI_TARGET)
	@echo "\n---> Installing CNI plugins:\n"
	mkdir -p $(ROOTFS)/opt/cni/bin
	mkdir -p $(ROOTFS)/etc/cni/net.d
	cd $(ASSETDIR) && tar -xzf $(CNI_TARBALL)
	cd $(ASSETDIR) && cp loopback $(ROOTFS)/opt/cni/bin/
	cp -afv ./cni-loopback.conf $(ROOTFS)/etc/cni/net.d/99-loopback.conf

$(CNI_TARGET):
	curl -L https://github.com/containernetworking/cni/releases/download/$(CNI_VER)/cni-$(CNI_VER).tgz -o "$@"

