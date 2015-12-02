# This makefile runs iside of docker's buildbox
# The following volumes are mounted and shared with the host:
ASSETS := /assets
ROOTFS := /rootfs
TARGETDIR := /targetdir

all: planet-bin
# common components:
	make -C $(ASSETS)/makefiles/base/network -f network.mk
	make -C $(ASSETS)/makefiles/base/docker -f docker.mk 
	make -C $(ASSETS)/makefiles/base/docker -f registry.mk
	make -C $(ASSETS)/makefiles/kubernetes -f kubernetes.mk
	make -C $(ASSETS)/makefiles/monit -f monitoring.mk
# master-image specific:
	make -C $(ASSETS)/makefiles/master/etcd -f etcd.mk
	make -C $(ASSETS)/makefiles/master/k8s-master -f k8s-master.mk
# shrink rootfs:
	make -e ROOTFS=$(ROOTFS) -C $(ASSETS)/makefiles -f shrink-rootfs.mk

planet-bin:
	go build -o $(ROOTFS)/usr/bin/planet github.com/gravitational/planet/tool/planet
