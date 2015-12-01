# This makefile runs inside the docker buildbox
# The following volumes are mounted and shared with the host:
ASSETS := /assets
ROOTFS := /rootfs
TARGETDIR := /targetdir

all: planet-bin
# common components:
	make -C $(ASSETS)/makefiles/base/network -f network.mk
	make -C $(ASSETS)/makefiles/base/docker -f docker.mk 
	make -C $(ASSETS)/makefiles/base/docker -f registry.mk
	make -C $(ASSETS)/makefiles/kubernetes -f kubernetes-dev.mk
	make -C $(ASSETS)/makefiles/monit -f monitoring.mk
# dev-image specific:
	make -C $(ASSETS)/makefiles/master/etcd -f etcd.mk
	make -C $(ASSETS)/makefiles/node/k8s-node -f k8s-node.mk
	make -C $(ASSETS)/makefiles/master/k8s-master -f k8s-master.mk
	make -C $(ASSETS)/makefiles/master/k8s-master -f k8s-e2e.mk
	make -C $(ASSETS)/makefiles/master/k8s-master -f registry.mk
# shrink rootfs:
	make -e ROOTFS=$(ROOTFS) -C $(ASSETS)/makefiles -f shrink-rootfs.mk

planet-bin:
	# Uncomment to build a completely static version of the planet binary (usual build command builds
	# an executable that depends on glibc due to dependency on docker
	# GOOS=linux GOARCH=amd64 go build --ldflags '-extldflags "-static"' -o $(ROOTFS)/usr/bin/planet github.com/gravitational/planet/tool/planet
	go build -o $(ROOTFS)/usr/bin/planet github.com/gravitational/planet/tool/planet
