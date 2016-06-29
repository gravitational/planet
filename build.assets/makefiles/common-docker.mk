# This makefile runs inside the docker buildbox
# The following volumes are mounted and shared with the host:
ASSETS := /assets
ROOTFS := /rootfs
TARGETDIR := /targetdir
ASSETDIR := /assetdir

.PHONY: all
all: $(ASSETDIR)/planet $(ASSETDIR)/docker-import
	make -C $(ASSETS)/makefiles/base/network -f network.mk
	make -C $(ASSETS)/makefiles/base/dns -f dns.mk
	make -C $(ASSETS)/makefiles/base/docker -f docker.mk 
	make -C $(ASSETS)/makefiles/base/agent -f agent.mk
	make -C $(ASSETS)/makefiles/kubernetes -f kubernetes.mk
	make -C $(ASSETS)/makefiles/etcd -f etcd.mk

$(ASSETDIR)/planet:
# Add to ldflags to compile a completely static version of the planet binary (w/o the glibc dependency)
# -ldflags '-extldflags "-static"'
	GOOS=linux GOARCH=amd64 go build -ldflags "$(PLANET_GO_LDFLAGS)" -o $@ github.com/gravitational/planet/tool/planet

$(ASSETDIR)/docker-import:
	GOOS=linux GOARCH=amd64 go build -ldflags "$(PLANET_GO_LDFLAGS)" -o $@ github.com/gravitational/planet/tool/docker-import
