# This makefile runs inside the buildbox container.
# The following volumes are mounted and shared with the host:
ASSETS := /assets
ROOTFS := /rootfs
TARGETDIR := /targetdir
ASSETDIR := /assetdir

all:
	make -C $(ASSETS)/makefiles -f common-docker.mk
	make -C $(ASSETS)/makefiles/base/docker -f registry.mk
	make -C $(ASSETS)/makefiles/master/k8s-master -f k8s-master.mk
	make -e ROOTFS=$(ROOTFS) -C $(ASSETS)/makefiles -f shrink-rootfs.mk
