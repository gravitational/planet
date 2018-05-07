# This makefile is used inside the buildbox container
SHELL:=/bin/bash
TARGETDIR:=$(BUILDDIR)/$(TARGET)
ASSETDIR:=$(BUILDDIR)/assets
ROOTFS:=$(TARGETDIR)/rootfs
CONTAINERNAME:=planet-base-build
TARBALL:=$(TARGETDIR)/planet.tar.gz
export
TMPFS_SIZE?=900m

.PHONY: all
all: $(ROOTFS)/bin/bash build planet-image

.PHONY: build
build:
	@echo -e "\n---> Launching 'buildbox' Docker container to build planet:\n"
	@mkdir -p $(ASSETDIR)
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
			ETCD_VER=$(ETCD_VER) \
			-C /assets/makefiles -f planet.mk
	$(MAKE) -C $(ASSETS)/makefiles/master/k8s-master -e -f containers.mk

.PHONY: planet-image
planet-image:
	cp $(ASSETS)/orbit.manifest.json $(TARGETDIR)
	cp $(ASSETDIR)/planet $(ROOTFS)/usr/bin/
	cp $(ASSETDIR)/docker-import $(ROOTFS)/usr/bin/
	@echo -e "\n---> Moving current symlink to $(TARGETDIR)\n"
	@rm -f $(BUILDDIR)/current
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
# populate Rootfs using docker image 'planet/base'
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
