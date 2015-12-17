# This makefile runs inside the docker buildbox
# The following volumes are mounted and shared with the host:
ASSETS := /assets
ROOTFS := /rootfs
TARGETDIR := /targetdir
ASSETDIR := /assetdir

all:
	make -C $(ASSETS)/makefiles -f common-docker.mk
	make -C $(ASSETS)/makefiles/node/k8s-node -f k8s-node.mk
	make -C $(ASSETS)/makefiles/node/etcdctl -f etcdctl.mk
# shrink rootfs:
	make -e ROOTFS=$(ROOTFS) -C $(ASSETS)/makefiles -f shrink-rootfs.mk
