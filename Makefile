.PHONY: all image etcd network k8s-master cube

BUILDDIR := $(abspath build)
export

all:
	mkdir -p $(BUILDDIR)
	ROOTFS=$(BUILDDIR)/rootfs $(MAKE) -C image -f image.mk
	ROOTFS=$(BUILDDIR)/rootfs $(MAKE) -C rkt -f rkt.mk
	ROOTFS=$(BUILDDIR)/rootfs $(MAKE) -C network -f network.mk

# at this point, $(BUILDDIR)/rootfs will contain the base Ubuntu image with rkt.
# create two copies of this rootfs, one for master, another for node
	mkdir -p $(BUILDDIR)/kube-master $(BUILDDIR)/kube-node
	cp -r $(BUILDDIR)/rootfs $(BUILDDIR)/kube-master
	cp -r $(BUILDDIR)/rootfs $(BUILDDIR)/kube-node

# build kube-master
	ROOTFS=$(BUILDDIR)/kube-master/rootfs $(MAKE) -C etcd -f etcd.mk
	ROOTFS=$(BUILDDIR)/kube-master/rootfs $(MAKE) -C k8s-master -f k8s-master.mk

# build kube-node
	ROOTFS=$(BUILDDIR)/kube-node/rootfs $(MAKE) -C k8s-node -f k8s-node.mk

cube:
	go install github.com/gravitational/cube/cube

run-master:
	sudo systemd-nspawn --boot --capability=all --register=true --uuid=51dbfeb9-59f9-4a5b-82db-0e5924202c63 --machine=kube-master -D $(BUILDDIR)/kube-master/rootfs --bind=/lib/modules

run-node: DIR := $(shell mktemp -d)
run-node:
# todo - this should be configurable
	echo -e 'MASTER_PRIVATE_IP="10.0.0.108"\n' > $(DIR)/master-private-ip
	sudo systemd-nspawn --boot --capability=all --register=true --uuid=a2e4b457-2844-4264-9de9-cb81d01cef53 --machine=kube-node -D $(BUILDDIR)/kube-node/rootfs --bind=/lib/modules --bind=$(DIR):/cluster-info --bind=/sys

enter-master:
	sudo nsenter --target $$(machinectl status kube-master | grep Leader | grep -Po '\d+') --pid --mount --uts --ipc --net /bin/bash

enter-node:
	sudo nsenter --target $$(machinectl status kube-node | grep Leader | grep -Po '\d+') --pid --mount --uts --ipc --net /bin/bash

enter:
	sudo nsenter --target $(PID) --pid --mount --uts --ipc --net /bin/bash


# IMPORTANT NOTES for installer
# * We need to set cloud provider for kubernetes
# * Flanneld needs NET_ADMIN and modpropbe, so we need to mount /lib/modules
# * Kube-node needs master private IP
# Have a unified way to generate environment for master and node in a consistent way and use one file everywhere
# what's the problem with udevd (turn it off probably)
# cgroups are mounted read only in systemd-nspawn, we should fix that by mounting them themselves I suppose.
