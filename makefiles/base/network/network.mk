.PHONY: all

VER := v0.5.3
ARCH := amd64
NAME := flannel
TARGET := $(NAME)-$(VER)
TARGET_TAR := $(TARGET)-linux-$(ARCH).tar.gz

all: $(BUILDDIR)/flanneld $(BUILDDIR)/setup-network-environment network.mk
	mkdir -p $(ROOTFS)/usr/bin
	cp -af $(BUILDDIR)/flanneld $(ROOTFS)/usr/bin
	cp -af $(BUILDDIR)/setup-network-environment $(ROOTFS)/usr/bin

	cp -af ./setup-network-environment.service $(ROOTFS)/lib/systemd/system
	cp -af ./flanneld.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/setup-network-environment.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
	ln -sf /lib/systemd/system/flanneld.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/

# script that allows waiting for etcd to come up
	mkdir -p $(ROOTFS)/usr/bin/scripts
	install -m 0755 ./wait-for-etcd.sh $(ROOTFS)/usr/bin/scripts

# script that sets up /etc/hosts and symlinks resolv.conf
	install -m 0755 ./setup-etc.sh $(ROOTFS)/usr/bin/scripts

# HACK: update ca-certificates should be done by deb2aci
	mkdir -p $(ROOTFS)/etc
	cp -af ./ca-certificates.conf $(ROOTFS)/etc



$(BUILDDIR)/flanneld: DIR := $(shell mktemp -d)
$(BUILDDIR)/flanneld: GOPATH := $(DIR)
$(BUILDDIR)/flanneld:
	mkdir -p $(DIR)/src/github.com/coreos
	cd $(DIR)/src/github.com/coreos && git clone https://github.com/coreos/flannel
	cd $(DIR)/src/github.com/coreos/flannel && git checkout $(VER)
	cd $(DIR)/src/github.com/coreos/flannel && go build -o flanneld .
	cp $(DIR)/src/github.com/coreos/flannel/flanneld $(BUILDDIR)
	rm -rf $(DIR)


$(BUILDDIR)/setup-network-environment: DIR := $(shell mktemp -d)
$(BUILDDIR)/setup-network-environment: GOPATH := $(DIR)
$(BUILDDIR)/setup-network-environment:
	mkdir -p $(DIR)/src/github.com/kelseyhightower
	cd $(DIR)/src/github.com/kelseyhightower && git clone https://github.com/kelseyhightower/setup-network-environment
	cd $(DIR)/src/github.com/kelseyhightower/setup-network-environment && godep go build .
	cp $(DIR)/src/github.com/kelseyhightower/setup-network-environment/setup-network-environment $(BUILDDIR)
	rm -rf $(DIR)
