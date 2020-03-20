# This makefile is used inside the buildbox container
SHELL := /bin/bash
TARGETDIR ?= $(BUILDDIR)/planet
ASSETDIR ?= $(BUILDDIR)/assets
ROOTFS ?= $(TARGETDIR)/rootfs
CONTAINERNAME ?= planet-base-build
TARBALL ?= $(BUILDDIR)/planet.tar.gz
export
TMPFS_SIZE ?= 900m
VER_UPDATES = ETCD_LATEST_VER KUBE_VER FLANNEL_VER DOCKER_VER HELM_VER COREDNS_VER

.PHONY: all
all: $(ROOTFS)/bin/bash build planet-image

$(ASSETDIR):
	@mkdir -p $(ASSETDIR)

.PHONY: build
build: | $(ASSETDIR)
	@echo -e "\n---> Launching 'buildbox' Docker container to build planet:\n"
	docker run -i -u $$(id -u) --rm=true \
		--volume=$(ASSETS):/assets \
		--volume=$(ROOTFS):/rootfs \
		--volume=$(TARGETDIR):/targetdir \
		--volume=$(ASSETDIR):/assetdir \
		--volume=$(PWD):/gopath/src/github.com/gravitational/planet \
		--env="ASSETS=/assets" \
		--env="ROOTFS=/rootfs" \
		--env="TARGETDIR=/targetdir" \
		--env="ASSETDIR=/assetdir" \
		planet/buildbox:latest \
		make -e \
			KUBE_VER=$(KUBE_VER) \
			FLANNEL_VER=$(FLANNEL_VER) \
			SERF_VER=$(SERF_VER) \
			ETCD_VER="$(ETCD_VER)" \
			ETCD_LATEST_VER=$(ETCD_LATEST_VER) \
			-C /assets/makefiles -f planet.mk
	$(MAKE) -C $(ASSETS)/makefiles/master/k8s-master -e -f containers.mk

.PHONY: planet-image
planet-image:
	cp $(ASSETDIR)/planet $(ROOTFS)/usr/bin/
	cp $(ASSETDIR)/docker-import $(ROOTFS)/usr/bin/
	cp $(ASSETS)/docker/os-rootfs/etc/planet/orbit.manifest.json $(TARGETDIR)/
	sed -i "s/REPLACE_ETCD_LATEST_VERSION/$(ETCD_LATEST_VER)/g" $(TARGETDIR)/orbit.manifest.json
	sed -i "s/REPLACE_KUBE_LATEST_VERSION/$(KUBE_VER)/g" $(TARGETDIR)/orbit.manifest.json
	sed -i "s/REPLACE_FLANNEL_LATEST_VERSION/$(FLANNEL_VER)/g" $(TARGETDIR)/orbit.manifest.json
	sed -i "s/REPLACE_DOCKER_LATEST_VERSION/$(DOCKER_VER)/g" $(TARGETDIR)/orbit.manifest.json
	sed -i "s/REPLACE_HELM_LATEST_VERSION/$(HELM_VER)/g" $(TARGETDIR)/orbit.manifest.json
	sed -i "s/REPLACE_COREDNS_LATEST_VERSION/$(COREDNS_VER)/g" $(TARGETDIR)/orbit.manifest.json
	@echo -e "\n---> Creating Planet image...\n"
	cd $(TARGETDIR) && fakeroot -- sh -c ' \
		chown -R 1000:1000 . ; \
		chown -R root:root rootfs/sbin/mount.* ; \
		tar -czf $(TARBALL) orbit.manifest.json rootfs'
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
		planet/buildbox:latest \
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
	if [[ $$(mount | grep $(ROOTFS)) ]]; then \
		sudo umount -f $(ROOTFS) 2>/dev/null ;\
	fi
# delete docker container we've used to create rootfs:
	if [[ $$(docker ps -a | grep $(CONTAINERNAME)) ]]; then \
		docker rm -f $(CONTAINERNAME) ;\
	fi

.PHONY: clean
clean: clean-rootfs
	rm -rf $(TARGETDIR)
