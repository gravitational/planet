# This makefile runs inside the docker buildbox
# The following volumes are mounted and shared with the host:
ASSETS := /assets
ROOTFS := /rootfs
TARGETDIR := /targetdir
ASSETDIR := /assetdir

all: $(ASSETDIR)/planet
	make -C $(ASSETS)/makefiles/base/network -f network.mk
	make -C $(ASSETS)/makefiles/base/docker -f docker.mk 
	make -C $(ASSETS)/makefiles/base/docker -f registry.mk
	make -C $(ASSETS)/makefiles/kubernetes -f kubernetes.mk
	make -C $(ASSETS)/makefiles/monit -f monitoring.mk

$(ASSETDIR)/planet:
	# Uncomment to build a completely static version of the planet binary (usual build command builds
	# an executable that depends on glibc due to dependency on docker
	# GOOS=linux GOARCH=amd64 go build --ldflags '-extldflags "-static"' -o $(ROOTFS)/usr/bin/planet github.com/gravitational/planet/tool/planet
	GOOS=linux GOARCH=amd64 go build -ldflags "$(PLANET_GO_LDFLAGS)" -o $@ github.com/gravitational/planet/tool/planet
