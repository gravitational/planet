.PHONY: all image etcd network k8s-master cube

MASTER_IP := 54.68.147.206
NODE_IP := 54.68.121.138

BUILDDIR := $(abspath build)
export

all:
	mkdir -p $(BUILDDIR)
	ROOTFS=$(BUILDDIR)/rootfs $(MAKE) -C image -f image.mk
	ROOTFS=$(BUILDDIR)/rootfs $(MAKE) -C network -f network.mk
	ROOTFS=$(BUILDDIR)/rootfs $(MAKE) -C docker -f docker.mk

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
	go install github.com/gravitational/cube/cube

# compile cube binary
	go install github.com/gravitational/cube/cube

# create tarballs 
	cp $(GOPATH)/bin/cube build/kube-master
	cp $(GOPATH)/bin/cube build/kube-node

	cd $(BUILDDIR) && tar -czf kube-master.tar.gz kube-master 
	cd $(BUILDDIR) && tar -czf kube-node.tar.gz kube-node

cube:
	go install github.com/gravitational/cube/cube

run-master:
	sudo $(shell which cube) build/kube-master/rootfs

run-node: DIR := $(shell mktemp -d)
run-node:
# todo - this should be configurable
	echo -e 'MASTER_PRIVATE_IP="10.0.0.108"\n' > $(DIR)/master-private-ip
	sudo systemd-nspawn --boot --capability=all --register=true --uuid=a2e4b457-2844-4264-9de9-cb81d01cef53 --machine=kube-node -D $(BUILDDIR)/kube-node/rootfs --bind=/lib/modules --bind=$(DIR):/cluster-info --bind=/sys

enter:
	sudo nsenter --target $(PID) --pid --mount --uts --ipc --net /bin/bash

enter-systemd:
	sudo nsenter --target $$(ps uax  | grep [/]bin/systemd | awk '{ print $$2 }') --pid --mount --uts --ipc --net /bin/bash

kill-systemd:
	sudo kill -9 $$(ps uax  | grep [/]bin/systemd | awk '{ print $$2 }')

login-master:
	ssh -i /home/alex/keys/aws/alex.pem ubuntu@$(MASTER_IP)

login-node:
	ssh -i /home/alex/keys/aws/alex.pem ubuntu@$(NODE_IP)

deploy-master:
	scp -i /home/alex/keys/aws/alex.pem  $(BUILDDIR)/kube-master.tar.gz ubuntu@$(MASTER_IP):/home/ubuntu

deploy-node:
	scp -i /home/alex/keys/aws/alex.pem  $(BUILDDIR)/kube-node.tar.gz ubuntu@$(NODE_IP):/home/ubuntu

deploy-cube:
	scp -i /home/alex/keys/aws/alex.pem  $(GOPATH)/bin/cube ubuntu@$(IP):/home/ubuntu/kube-master/

deploy-nsenter:
	scp -i /home/alex/keys/aws/alex.pem /usr/bin/nsenter ubuntu@$(IP):/home/ubuntu/


# IMPORTANT NOTES for installer
# * We need to set cloud provider for kubernetes
# * Flanneld needs NET_ADMIN and modpropbe, so we need to mount /lib/modules
# * Kube-node needs master private IP
# Have a unified way to generate environment for master and node in a consistent way and use one file everywhere
# what's the problem with udevd (turn it off probably)
# cgroups are mounted read only in systemd-nspawn, we should fix that by mounting them themselves I suppose.
