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
	# Add to ldflags to compile a completely static version of the planet binary (w/o the glibc dependency)
	# -ldflags '-extldflags "-static"'
	GOOS=linux GOARCH=amd64 go build -ldflags "$(PLANET_GO_LDFLAGS)" -o $@ github.com/gravitational/planet/tool/planet
