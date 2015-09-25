SHELL:=/bin/bash
.PHONY: build os base buildbox dev master node

PWD := $(shell pwd)
ASSETS := $(PWD)/build.assets
BUILDDIR ?= $(PWD)/build
BUILDDIR := $(shell realpath $(BUILDDIR))
PLANETVER:=0.01
export

build: 
	go install github.com/gravitational/planet/tool/planet
	@ln -sf $$GOPATH/bin/planet $(BUILDDIR)/current/rootfs/usr/bin/planet

# Builds the base Docker image (bare bones OS). Everything else is based on. 
# Debian stable + configured locales. 
os: 
	echo -e "\\n---> Making Planet/OS (Debian) Docker image...\\n"
	$(MAKE) -e BUILDIMAGE=planet/os DOCKERFILE=os.dockerfile make-docker-image

# Builds on top of "bare OS" image by adding components that every Kubernetes/planet node
# needs (like bridge-utils or kmod)
base: os
	echo -e "\\n---> Making Planet/Base Docker image based on Planet/OS...\\n"
	$(MAKE) -e BUILDIMAGE=planet/base DOCKERFILE=base.dockerfile make-docker-image

# Builds a "buildbox" docker image. Actual building is done inside of Docker, and this
# image is used as a build box. It contains dev tools (Golang, make, git, vi, etc)
buildbox: base
	echo -e "\\n---> Making Planet/BuilBox Docker image to be used for building:\\n" ;\
	$(MAKE) -e BUILDIMAGE=planet/buildbox DOCKERFILE=buildbox.dockerfile make-docker-image

make-docker-image:
	@if [[ ! $$(docker images | grep $(BUILDIMAGE)) ]]; then \
		cd $(ASSETS)/docker; docker build --no-cache=true -t $(BUILDIMAGE) -f $(DOCKERFILE) . ;\
	else \
		echo "$(BUILDIMAGE) already exists. Run 'docker rmi $(BUILDIMAGE)' to rebuild" ;\
	fi

# Makes a "developer" image, with _all_ parts of Kubernetes installed
dev: buildbox
	$(MAKE) -C $(ASSETS)/makefiles -e TARGET=dev -f buildbox.mk

# Makes a "master" image, with only master components of Kubernetes installed
master: buildbox
	$(MAKE) -C $(ASSETS)/makefiles -e TARGET=master -f buildbox.mk

# Makes a "node" image, with only node components of Kubernetes installed
node: buildbox
	$(MAKE) -C $(ASSETS)/makefiles -e TARGET=node -f buildbox.mk

# remvoes all build aftifacts 
clean: dev-clean master-clean node-clean
dev-clean:
	$(MAKE) -C $(ASSETS)/makefiles -e TARGET=dev -f buildbox.mk clean
node-clean:
	$(MAKE) -C $(ASSETS)/makefiles -e TARGET=node -f buildbox.mk clean
master-clean:
	$(MAKE) -C $(ASSETS)/makefiles -e TARGET=master -f buildbox.mk clean


dev-start: prepare-to-run
	cd $(BUILDDIR)/current && sudo rootfs/usr/bin/planet start\
		--role=master\
		--role=node\
		--volume=/var/planet/etcd:/ext/etcd\
		--volume=/var/planet/registry:/ext/registry\
		--volume=/var/planet/docker:/ext/docker

node-start: prepare-to-run
	cd $(BUILDDIR)/current && sudo rootfs/usr/bin/planet start\
		--role=node\
		--volume=/var/planet/etcd:/ext/etcd\
		--volume=/var/planet/registry:/ext/registry\
		--volume=/var/planet/docker:/ext/docker

master-start: prepare-to-run
	cd $(BUILDDIR)/current && sudo rootfs/usr/bin/planet start\
		--role=master\
		--volume=/var/planet/etcd:/ext/etcd\
		--volume=/var/planet/registry:/ext/registry\
		--volume=/var/planet/docker:/ext/docker

stop:
	cd $(BUILDDIR)/current && sudo rootfs/usr/bin/planet --debug stop

enter:
	cd $(BUILDDIR)/current && sudo rootfs/usr/bin/planet enter --debug /bin/bash

remove-godeps:
	rm -rf Godeps/
	find . -iregex .*go | xargs sed -i 's:".*Godeps/_workspace/src/:":g'

prepare-to-run:
	@sudo mkdir -p /var/planet/registry /var/planet/etcd /var/planet/docker 
	@sudo chown $$USER:$$USER /var/planet/etcd -R

clean-containers: DIRTY:=$(shell docker ps --all | grep "planet" | awk '{print $$1}')
clean-containers:
	@echo -e "Removing dead Docker/planet containers...\n"
	-@if [ ! -z "$(DIRTY)" ] ; then \
		docker rm -f $(DIRTY) ;\
	fi

clean-docker-images: DIRTY:=$(shell docker images | grep "planet/" | awk '{print $$3}')
clean-docker-images: clean-containers
	@echo -e "Removing old Docker/planet images...\n"
	-@if [ ! -z "$(DIRTY)" ] ; then \
		docker rmi -f $(DIRTY) ;\
	fi

