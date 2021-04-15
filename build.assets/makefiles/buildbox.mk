# This makefile is used inside the buildbox container
TARGETDIR ?= $(BUILDDIR)/planet
ASSETDIR ?= $(BUILDDIR)/assets
ROOTFS ?= $(TARGETDIR)/rootfs
CONTAINERNAME ?= planet-base-build
TARBALL ?= $(BUILDDIR)/planet.tar.gz
PLANET_BUILD_TAG ?= $(shell git describe --tags)
BUILDBOX_NAME ?= planet/buildbox
BUILDBOX_IMAGE ?= $(BUILDBOX_NAME):$(PLANET_BUILD_TAG)
GO ?= go
GOCACHE_DOCKER_OPTIONS ?=
GOCACHE ?= $(shell go env GOCACHE 2>/dev/null || echo $${HOME}/.cache/go-build)
ifdef GOCACHE_ENABLED
GOCACHE_DOCKER_OPTIONS = --volume $(GOCACHE):$(GOCACHE) --env "GOCACHE=$(GOCACHE)"
endif

export

OS := $(shell uname | tr '[:upper:]' '[:lower:]')
ARCH := $(shell uname -m)

define replace
	sed -i "$1" $2
endef

ifeq ($(OS),darwin)
define replace
	sed -i '' "$1" $2
endef
endif

TMPFS_SIZE ?= 900m
VER_UPDATES = ETCD_LATEST_VER KUBE_VER FLANNEL_VER DOCKER_VER HELM_VER HELM3_VER COREDNS_VER NODE_PROBLEM_DETECTOR_VER

.PHONY: all
all: $(ROOTFS)/bin/bash build planet-image

$(ASSETDIR):
	@mkdir -p $(ASSETDIR)

.PHONY: build
build: | $(ASSETDIR) $(GOCACHE)
	@echo -e "\n---> Launching 'buildbox' Docker container to build planet:\n"
	docker run -i -u $$(id -u) --rm=true \
		--volume=$(ASSETS):/assets \
		--volume=$(ROOTFS):/rootfs \
		--volume=$(TARGETDIR):/targetdir \
		--volume=$(ASSETDIR):/assetdir \
		--volume=$(CURRENT_DIR):/gopath/src/github.com/gravitational/planet \
		--env="ASSETS=/assets" \
		--env="ROOTFS=/rootfs" \
		--env="TARGETDIR=/targetdir" \
		--env="ASSETDIR=/assetdir" \
		$(GOCACHE_DOCKER_OPTIONS) \
		$(BUILDBOX_IMAGE) \
		dumb-init make -e \
			KUBE_VER=$(KUBE_VER) \
			HELM_VER=$(HELM_VER) \
			HELM3_VER=$(HELM3_VER) \
			COREDNS_VER=$(COREDNS_VER) \
			CNI_VER=$(CNI_VER) \
			FLANNEL_VER=$(FLANNEL_VER) \
			SERF_VER=$(SERF_VER) \
			NODE_PROBLEM_DETECTOR_VER=$(NODE_PROBLEM_DETECTOR_VER) \
			DOCKER_VER=$(DOCKER_VER) \
			ETCD_VER="$(ETCD_VER)" \
			ETCD_LATEST_VER=$(ETCD_LATEST_VER) \
			PLANET_UID=$(PLANET_UID) \
			PLANET_GID=$(PLANET_GID) \
			-C /assets/makefiles -f planet.mk
	$(MAKE) -C $(ASSETS)/makefiles/master/k8s-master -e -f containers.mk

$(GOCACHE):
	mkdir -p $@

.PHONY: planet-image
planet-image:
	cp $(ASSETDIR)/planet $(ROOTFS)/usr/bin/
	cp $(ASSETDIR)/docker-import $(ROOTFS)/usr/bin/
	cp $(ASSETS)/docker/os-rootfs/etc/planet/orbit.manifest.json $(TARGETDIR)/
	$(call replace,"s/REPLACE_ETCD_LATEST_VERSION/$(ETCD_LATEST_VER)/g",$(TARGETDIR)/orbit.manifest.json)
	$(call replace,"s/REPLACE_KUBE_LATEST_VERSION/$(KUBE_VER)/g",$(TARGETDIR)/orbit.manifest.json)
	$(call replace,"s/REPLACE_FLANNEL_LATEST_VERSION/$(FLANNEL_VER)/g",$(TARGETDIR)/orbit.manifest.json)
	$(call replace,"s/REPLACE_DOCKER_LATEST_VERSION/$(DOCKER_VER)/g",$(TARGETDIR)/orbit.manifest.json)
	$(call replace,"s/REPLACE_HELM_LATEST_VERSION/$(HELM_VER)/g",$(TARGETDIR)/orbit.manifest.json)
	$(call replace,"s/REPLACE_HELM3_LATEST_VERSION/$(HELM3_VER)/g",$(TARGETDIR)/orbit.manifest.json)
	$(call replace,"s/REPLACE_COREDNS_LATEST_VERSION/$(COREDNS_VER)/g",$(TARGETDIR)/orbit.manifest.json)
	$(call replace,"s/REPLACE_NODE_PROBLEM_DETECTOR_LATEST_VERSION/$(NODE_PROBLEM_DETECTOR_VER)/g",$(TARGETDIR)/orbit.manifest.json)
	cp $(TARGETDIR)/orbit.manifest.json $(ROOTFS)/etc/planet/
	@echo -e "\n---> Creating Planet image...\n"
	$(GO) run github.com/gravitational/planet/tool/create-tarball/... $(TARGETDIR) $(BUILDDIR)/planet.tar.gz
	@echo -e "\nDone --> $(TARBALL)"

.PHONY: enter-buildbox
enter-buildbox:
	docker run -ti -u $$(id -u) --rm=true \
		--volume=$(ASSETS):/assets \
		--volume=$(ROOTFS):/rootfs \
		--volume=$(TARGETDIR):/targetdir \
		--volume=$(ASSETDIR):/assetdir \
		--volume=$(PWD):/gopath/src/github.com/gravitational/planet \
		--env="ASSETS=/assets" \
		--env="ROOTFS=/rootfs" \
		--env="TARGETDIR=/targetdir" \
		--env="ASSETDIR=/assetdir" \
		$(BUILDBOX_IMAGE) \
		/bin/bash

$(ROOTFS)/bin/bash: clean-rootfs
	@echo -e "\n---> Creating RootFS for Planet image:\n"
	@mkdir -p $(ROOTFS)
# if MEMROOTFS environment variable is set, create rootfs in RAM (to speed up interative development)
	if [ ! -z $$MEMROOTFS ]; then \
	  sudo mount -t tmpfs -o size=$(TMPFS_SIZE) tmpfs $(ROOTFS) ;\
	fi
# populate rootfs using docker image 'planet/base'
	docker create --name=$(CONTAINERNAME) $(PLANET_IMAGE)
	@echo "Exporting base Docker image into a fresh RootFS into $(ROOTFS)...."
	cd $(ROOTFS) && docker export $(CONTAINERNAME) | tar -x


.PHONY: clean-rootfs
clean-rootfs:
# umount tmps volume for rootfs:
	if mount | grep $(ROOTFS); then \
		sudo umount -f $(ROOTFS) 2>/dev/null ;\
	fi
# delete docker container we've used to create rootfs:
	if docker ps -a | grep $(CONTAINERNAME); then \
		docker rm -f $(CONTAINERNAME) ;\
	fi

.PHONY: clean
clean: clean-rootfs
	rm -rf $(TARGETDIR)
