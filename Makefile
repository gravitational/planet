# Quick Start
# -----------
# make production:
#     CD/CD build of Planet. This is what's used by Jenkins builds and this
#     is what gets released to customers.
#
# make:
#     builds your changes and updates planet binary in
#     build/current/rootfs/usr/bin/planet
#
# make start:
#     starts Planet from build/dev/rootfs/usr/bin/planet
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
#     8. Last, rootfs is stored into a ready for distribution tarball.
#
.DEFAULT_GOAL := all

SHELL := /bin/bash
PWD := $(shell pwd)
ASSETS := $(PWD)/build.assets
BUILD_ASSETS := $(PWD)/build/assets
BUILDDIR ?= $(PWD)/build
BUILDDIR := $(shell realpath $(BUILDDIR))
OUTPUTDIR := $(BUILDDIR)/planet

KUBE_VER ?= v1.9.6
SECCOMP_VER ?=  2.3.1-2.1
DOCKER_VER ?= 17.03.2
# we currently use our own flannel fork: gravitational/flannel
FLANNEL_VER ?= master
HELM_VER := v2.8.1

# ETCD Versions to include in the release
# This list needs to include every version of etcd that we can upgrade from + latest
ETCD_VER := v2.3.8 v3.3.4
# This is the version of etcd we should upgrade to (from the version list)
ETCD_LATEST_VER := v3.3.4

BUILDBOX_GO_VER ?= 1.10.1
PLANET_BUILD_TAG ?= $(shell git describe --tags)
PLANET_IMAGE_NAME ?= planet/base
PLANET_IMAGE ?= $(PLANET_IMAGE_NAME):$(PLANET_BUILD_TAG)
export

PUBLIC_IP ?= 127.0.0.1
PLANET_PACKAGE_PATH := $(PWD)
PLANET_PACKAGE := github.com/gravitational/planet
PLANET_VERSION_PACKAGE_PATH := $(PLANET_PACKAGE)/Godeps/_workspace/src/github.com/gravitational/version
GO_FILES := $(shell find . -type f -name '*.go' -not -path "./vendor/*" -not -path "./build/*")
# Space separated patterns of packages to skip
IGNORED_PACKAGES := /vendor build/

.PHONY: all
all: production image

# 'make build' compiles the Go portion of Planet, meant for quick & iterative
# development on an _already built image_. You need to build an image first, for
# example with "make dev"
.PHONY: build
build: $(BUILD_ASSETS)/planet $(BUILDDIR)/planet.tar.gz
	cp -f $< $(OUTPUTDIR)/rootfs/usr/bin/

# Deploys the build artifacts to Amazon S3
.PHONY: deploy
deploy:
	$(MAKE) -C $(ASSETS)/makefiles/deploy deploy

.PHONY: production
production: $(BUILDDIR)/planet.tar.gz

.PHONY: image
image:
	$(MAKE) -C $(ASSETS)/makefiles/deploy image

$(BUILD_ASSETS)/planet:
	GOOS=linux GOARCH=amd64 \
	     go install -ldflags "$(PLANET_GO_LDFLAGS)" \
	     $(PLANET_PACKAGE)/tool/planet -o $@

$(BUILDDIR)/planet.tar.gz: buildbox Makefile $(wildcard build.assets/**/*) $(GO_FILES)
	$(MAKE) -C $(ASSETS)/makefiles -e \
		PLANET_BASE_IMAGE=$(PLANET_IMAGE) \
		TARGETDIR=$(OUTPUTDIR) \
		-f buildbox.mk

.PHONY: enter-buildbox
enter-buildbox:
	$(MAKE) -C $(ASSETS)/makefiles -e -f buildbox.mk enter-buildbox

# Run package tests
.PHONY: test
test: remove-temp-files
	go test -race -v -test.parallel=1 ./tool/... ./lib/...

.PHONY: test-package-with-etcd
test-package-with-etcd: remove-temp-files
	PLANET_TEST_ETCD_NODES=http://127.0.0.1:4001 go test -v -test.parallel=0 ./$(p)

.PHONY: remove-temp-files
remove-temp-files:
	find . -name flymake_* -delete

# Start the planet container locally
.PHONY: start
start: build prepare-to-run
	cd $(OUTPUTDIR) && sudo rootfs/usr/bin/planet start \
		--debug \
		--etcd-member-name=local-planet \
		--secrets-dir=/var/planet/state \
		--public-ip=$(PUBLIC_IP) \
		--role=master \
		--service-uid=1000 \
		--initial-cluster=local-planet:$(PUBLIC_IP) \
		--volume=/var/planet/agent:/ext/agent \
		--volume=/var/planet/etcd:/ext/etcd \
		--volume=/var/planet/registry:/ext/registry \
		--volume=/var/planet/docker:/ext/docker

# Stop the running planet container
.PHONY: stop
stop:
	cd $(OUTPUTDIR) && sudo rootfs/usr/bin/planet --debug stop

# Enter the running planet container
.PHONY: enter
enter:
	cd $(OUTPUTDIR) && sudo rootfs/usr/bin/planet enter --debug /bin/bash

# Build the base Docker image everything else is based on.
.PHONY: os
os:
	@echo -e "\n---> Making Planet/OS (Debian) Docker image...\n"
	$(MAKE) -e BUILDIMAGE=planet/os DOCKERFILE=os.dockerfile make-docker-image

# Build the image with components required for running a Kubernetes node
.PHONY: base
base: os
	@echo -e "\n---> Making Planet/Base Docker image based on Planet/OS...\n"
	$(MAKE) -e BUILDIMAGE=$(PLANET_IMAGE) DOCKERFILE=base.dockerfile \
		EXTRA_ARGS="--build-arg SECCOMP_VER=$(SECCOMP_VER) --build-arg DOCKER_VER=$(DOCKER_VER) --build-arg HELM_VER=$(HELM_VER)" \
		make-docker-image

# Build a container used for building the planet image
.PHONY: buildbox
buildbox: base
	@echo -e "\n---> Making Planet/BuildBox Docker image:\n" ;\
	$(MAKE) -e BUILDIMAGE=planet/buildbox \
		DOCKERFILE=buildbox.dockerfile \
		EXTRA_ARGS="--build-arg GOVERSION=$(BUILDBOX_GO_VER) --build-arg PLANET_BASE_IMAGE=$(PLANET_IMAGE)" \
		make-docker-image

# Remove build artifacts
.PHONY: clean
clean:
	$(MAKE) -C $(ASSETS)/makefiles -f buildbox.mk clean
	rm -rf $(BUILDDIR)

# internal use:
.PHONY: make-docker-image
make-docker-image:
	cd $(ASSETS)/docker; docker build $(EXTRA_ARGS) -t $(BUILDIMAGE) -f $(DOCKERFILE) .

.PHONY: remove-godeps
remove-godeps:
	rm -rf Godeps/
	find . -iregex .*go | xargs sed -i 's:".*Godeps/_workspace/src/:":g'

.PHONY: prepare-to-run
prepare-to-run: build
	@sudo mkdir -p /var/planet/registry /var/planet/etcd /var/planet/docker
	@sudo chown $$USER:$$USER /var/planet/etcd -R
	@cp -f $(BUILD_ASSETS)/planet $(OUTPUTDIR)/rootfs/usr/bin/planet

.PHONY: clean-containers
clean-containers:
	@echo -e "\n---> Removing dead Docker/planet containers...\n"
	DEADCONTAINTERS=$$(docker ps --all | grep "planet" | awk '{print $$1}') ;\
	if [ ! -z "$$DEADCONTAINTERS" ] ; then \
		docker rm -f $$DEADCONTAINTERS ;\
	fi

.PHONY: clean-images
clean-images: clean-containers
	@echo -e "\n---> Removing old Docker/planet images...\n"
	DEADIMAGES=$$(docker images | grep "planet/" | awk '{print $$3}') ;\
	if [ ! -z "$$DEADIMAGES" ] ; then \
		docker rmi -f $$DEADIMAGES ;\
	fi

.PHONY: fix-logrus
fix-logrus:
	find vendor -type f -print0 | xargs -0 sed -i 's/Sirupsen/sirupsen/g'

.PHONY: get-version
get-version:
	@echo $(PLANET_BUILD_TAG)
