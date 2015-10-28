# Quick Start
# -----------
# make dev: 
#     builds 'development' image of Planet, stores output in build/dev and 
#     points build/current symlink to it. 
#
# make: 
#     builds your changes and updates planet binary in 
#     build/current/rootfs/usr/bin/planet
#
# make dev-start:
#     starts Planet from build/dev/rootfs/usr/bin/planet
#
# make test:
#     starts Planet in self-test mode
#     requires `make dev` and `make dev-start`
#
# Build Steps
# -----------
# The sequence of steps the build process takes:
#     1. Make 'os' Docker image: the empty Debian 8 image.
#     2. Make 'base' image on top of 'os' (Debian + our additions)
#     3. Make 'buildbox' image on top of 'os'. Used for building, 
#        not part of the Planet image.
#     4. Build various components (flannel, etcd, k8s, etc) inside
#        of the 'buildbox' based on inputs (master/node/dev)
#     5. Store everything inside a temporary Docker image based on 'base'
#     6. Export the root FS of that image into build/current/rootfs
#     7. build/current/rootfs is basically the output of the build.
#     8. Later, root FS is tarballed and ready for distribution.
#
SHELL:=/bin/bash
.PHONY: build os base buildbox dev master node testbox test

PWD := $(shell pwd)
ASSETS := $(PWD)/build.assets
BUILDDIR ?= $(PWD)/build
BUILDDIR := $(shell realpath $(BUILDDIR))
PLANETVER:=0.01
export

# 'make build' compiles GO portion of Planet, meant for quick & iterative 
# devlopment on an _already built image_. You need to build an image first, for 
# example with "make dev"
build: $(BUILDDIR)/current
	go install github.com/gravitational/planet/tool/planet
	cp -f $$GOPATH/bin/planet $(BUILDDIR)/current/planet 
	rm -f $(BUILDDIR)/current/rootfs/usr/bin/planet
	cp -f $$GOPATH/bin/planet $(BUILDDIR)/current/rootfs/usr/bin/planet

# Makes a "developer" image, with _all_ parts of Kubernetes installed
dev: buildbox
	$(MAKE) -C $(ASSETS)/makefiles -e TARGET=dev -f buildbox.mk

# Makes a "master" image, with only master components of Kubernetes installed
master: buildbox
	$(MAKE) -C $(ASSETS)/makefiles -e TARGET=master -f buildbox.mk

# Makes a "node" image, with only node components of Kubernetes installed
node: buildbox
	$(MAKE) -C $(ASSETS)/makefiles -e TARGET=node -f buildbox.mk

# Runs end-to-end tests in the specific environment
test: testbox
	$(MAKE) -C $(ASSETS)/makefiles -e TARGET=dev SPEC=$(SPEC) -f test.mk

# Starts "planet-dev" build. It needs to be built first with "make dev"
dev-start: prepare-to-run
	cd $(BUILDDIR)/current && sudo rootfs/usr/bin/planet start\
		--debug\
		--role=master\
		--role=node\
		--volume=/var/planet/etcd:/ext/etcd\
		--volume=/var/planet/registry:/ext/registry\
		--volume=/var/planet/docker:/ext/docker

# Starts "planet-node" image. It needs to be built first with "make node"
node-start: prepare-to-run
	cd $(BUILDDIR)/current && sudo rootfs/usr/bin/planet start\
		--role=node\
		--volume=/var/planet/etcd:/ext/etcd\
		--volume=/var/planet/registry:/ext/registry\
		--volume=/var/planet/docker:/ext/docker

# Starts "planet-master" image. It needs to be built first with "make master"
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

# Builds the base Docker image (bare bones OS). Everything else is based on. 
# Debian stable + configured locales. 
os: 
	@echo -e "\n---> Making Planet/OS (Debian) Docker image...\n"
	$(MAKE) -e BUILDIMAGE=planet/os DOCKERFILE=os.dockerfile make-docker-image

# Builds on top of "bare OS" image by adding components that every Kubernetes/planet node
# needs (like bridge-utils or kmod)
base: os
	@echo -e "\n---> Making Planet/Base Docker image based on Planet/OS...\n"
	$(MAKE) -e BUILDIMAGE=planet/base DOCKERFILE=base.dockerfile make-docker-image

# Builds a "buildbox" docker image. Actual building is done inside of Docker, and this
# image is used as a build box. It contains dev tools (Golang, make, git, vi, etc)
buildbox: base
	@echo -e "\n---> Making Planet/BuildBox Docker image to be used for building:\n" ;\
	$(MAKE) -e BUILDIMAGE=planet/buildbox DOCKERFILE=buildbox.dockerfile make-docker-image

# Builds a "testbox" image used during e2e testing.
testbox:
	@echo -e "\n---> Making planet/testbox image for e2e testing:\n" ;\
	$(MAKE) -e BUILDIMAGE=planet/testbox DOCKERFILE=testbox.dockerfile make-docker-image

# removes all build aftifacts 
clean: dev-clean master-clean node-clean test-clean
dev-clean:
	$(MAKE) -C $(ASSETS)/makefiles -e TARGET=dev -f buildbox.mk clean
node-clean:
	$(MAKE) -C $(ASSETS)/makefiles -e TARGET=node -f buildbox.mk clean
master-clean:
	$(MAKE) -C $(ASSETS)/makefiles -e TARGET=master -f buildbox.mk clean
test-clean:
	$(MAKE) -C $(ASSETS)/makefiles -e TARGET=dev -f testbox.mk clean

# internal use:
make-docker-image:
	@if [[ ! $$(docker images | grep $(BUILDIMAGE)) ]]; then \
		cd $(ASSETS)/docker; docker build --no-cache -t $(BUILDIMAGE) -f $(DOCKERFILE) . ;\
	else \
		echo "$(BUILDIMAGE) already exists. Run 'docker rmi $(BUILDIMAGE)' to rebuild" ;\
	fi

remove-godeps:
	rm -rf Godeps/
	find . -iregex .*go | xargs sed -i 's:".*Godeps/_workspace/src/:":g'

prepare-to-run: 
	@sudo mkdir -p /var/planet/registry /var/planet/etcd /var/planet/docker 
	@sudo chown $$USER:$$USER /var/planet/etcd -R
	@cp -f $(BUILDDIR)/current/planet $(BUILDDIR)/current/rootfs/usr/bin/planet

clean-containers:
	@echo -e "\n---> Removing dead Docker/planet containers...\n"
	DEADCONTAINTERS=$$(docker ps --all | grep "planet" | awk '{print $$1}') ;\
	if [ ! -z "$$DEADCONTAINTERS" ] ; then \
		docker rm -f $$DEADCONTAINTERS ;\
	fi

clean-images: clean-containers
	@echo -e "\n---> Removing old Docker/planet images...\n"
	DEADIMAGES=$$(docker images | grep "planet/" | awk '{print $$3}') ;\
	if [ ! -z "$$DEADIMAGES" ] ; then \
		docker rmi -f $$DEADIMAGES ;\
	fi

$(BUILDDIR)/current:
	@echo "You need to build the full image first. Run \"make dev\""
