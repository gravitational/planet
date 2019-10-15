# This makefile runs inside the docker buildbox
# The following volumes are mounted and shared with the host:
ASSETS := /assets
ROOTFS := /rootfs
TARGETDIR := /targetdir
ASSETDIR := /assetdir
LINKFLAGS_TAG := master
PLANET_PKG_PATH := /gopath/src/github.com/gravitational/planet

.PHONY: all
all:
	make -C $(ASSETS)/makefiles/base/systemd
	make -C $(ASSETS)/makefiles/base/network -f network.mk
	make -C $(ASSETS)/makefiles/base/node-problem-detector -f node-problem-detector.mk
	make -C $(ASSETS)/makefiles/base/dns -f dns.mk
	make -C $(ASSETS)/makefiles/base/docker -f docker.mk
	make -C $(ASSETS)/makefiles/base/agent -f agent.mk
	make -C $(ASSETS)/makefiles/kubernetes -f kubernetes.mk
	make -C $(ASSETS)/makefiles/etcd -f etcd.mk
