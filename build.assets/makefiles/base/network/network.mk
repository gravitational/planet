.PHONY: all

VER := v0.5.3
ARCH := amd64
NAME := flannel
TARGET := $(NAME)-$(VER)
TARGET_TAR := $(TARGET)-linux-$(ARCH).tar.gz
BINARIES := $(ASSETDIR)/flanneld-$(VER)

all: $(BINARIES) network.mk
	@echo "\\n---> Installing Flannel and preparing network stack for Kubernetes:\\n"
	mkdir -p $(ASSETDIR)
	cp -af $(BINARIES) $(ROOTFS)/usr/bin/flanneld

	cp -af ./flanneld.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/flanneld.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/

# script that allows waiting for etcd to come up
	mkdir -p $(ROOTFS)/usr/bin/scripts
	install -m 0755 ./wait-for-etcd.sh $(ROOTFS)/usr/bin/scripts

# script that sets up /etc/hosts and symlinks resolv.conf
	install -m 0755 ./setup-etc.sh $(ROOTFS)/usr/bin/scripts

$(BINARIES): DIR := $(shell mktemp -d)
$(BINARIES): GOPATH := $(DIR)
$(BINARIES):
	mkdir -p $(DIR)/src/github.com/coreos
	cd $(DIR)/src/github.com/coreos && git clone https://github.com/coreos/flannel -b $(VER) --depth 1
	cd $(DIR)/src/github.com/coreos/flannel && go build -o flanneld .
	cp $(DIR)/src/github.com/coreos/flannel/flanneld $@
	rm -rf $(DIR)
