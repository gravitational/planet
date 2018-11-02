.PHONY: all

ARCH := amd64
NAME := flannel
TARGET := $(NAME)-$(FLANNEL_VER)
TARGET_TAR := $(TARGET)-linux-$(ARCH).tar.gz
BINARIES := $(ASSETDIR)/flanneld-$(FLANNEL_VER)

all: $(BINARIES) network.mk
	@echo "\\n---> Installing Flannel and preparing network stack for Kubernetes:\\n"
	mkdir -p $(ASSETDIR)
	cp -af $(BINARIES) $(ROOTFS)/usr/bin/flanneld

	cp -af ./flanneld.service $(ROOTFS)/lib/systemd/system

# Setup CNI and include flannel as a plugin
	mkdir -p $(ROOTFS)/etc/cni/net.d/ $(ROOTFS)/opt/cni/bin
	curl -L --retry 5 https://github.com/containernetworking/plugins/releases/download/v0.7.1/cni-plugins-amd64-v0.7.1.tgz \
    | tar -xz -C $(ROOTFS)/opt/cni/bin ./bridge ./loopback ./host-local ./portmap ./tuning
	ln -sf /usr/bin/flanneld $(ROOTFS)/opt/cni/bin/flanneld

# script that allows waiting for etcd to come up
	mkdir -p $(ROOTFS)/usr/bin/scripts
	install -m 0755 ./wait-for-etcd.sh $(ROOTFS)/usr/bin/scripts

# script that sets up /etc/hosts and symlinks resolv.conf
	install -m 0755 ./setup-etc.sh $(ROOTFS)/usr/bin/scripts

$(BINARIES): DIR := $(shell mktemp -d)
$(BINARIES): GOPATH := $(DIR)
$(BINARIES):
	mkdir -p $(DIR)/src/github.com/coreos
	cd $(DIR)/src/github.com/coreos && git clone https://github.com/gravitational/flannel -b $(FLANNEL_VER) --depth 1
	cd $(DIR)/src/github.com/coreos/flannel && go build -o flanneld .
	cp $(DIR)/src/github.com/coreos/flannel/flanneld $@
	rm -rf $(DIR)
