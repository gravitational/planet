# This makefile runs inside the docker buildbox
# The following volumes are mounted and shared with the host:
ASSETS := /assets
ROOTFS := /rootfs
TARGETDIR := /targetdir
ASSETDIR := /assetdir
LINKFLAGS_TAG := master
PLANET_PKG_PATH := /gopath/src/github.com/gravitational/planet
PLANET_BUILDFLAGS := -tags 'selinux'

.PHONY: all
all: common-docker.mk $(ASSETDIR)/planet $(ASSETDIR)/docker-import
	make -C $(ASSETS)/makefiles/base/systemd
	make -C $(ASSETS)/makefiles/base/network -f network.mk
	make -C $(ASSETS)/makefiles/base/dns -f dns.mk
	make -C $(ASSETS)/makefiles/base/docker -f docker.mk
	make -C $(ASSETS)/makefiles/base/agent -f agent.mk
	make -C $(ASSETS)/makefiles/kubernetes -f kubernetes.mk
	make -C $(ASSETS)/makefiles/etcd -f etcd.mk

$(ASSETDIR)/planet: flags
# Add to ldflags to compile a completely static version of the planet binary (w/o the glibc dependency)
# -ldflags '-extldflags "-static"'
	GOOS=linux GOARCH=amd64 go build -ldflags $(PLANET_LINKFLAGS) $(PLANET_BUILDFLAGS) -o $@ github.com/gravitational/planet/tool/planet

$(ASSETDIR)/docker-import:
	GOOS=linux GOARCH=amd64 go build -ldflags "$(PLANET_GO_LDFLAGS)" -o $@ github.com/gravitational/planet/tool/docker-import

.PHONY: flags
flags:
	go install github.com/gravitational/version/cmd/linkflags
	$(eval PLANET_LINKFLAGS := "$(shell linkflags -pkg=$(PLANET_PKG_PATH) -verpkg=github.com/gravitational/planet/vendor/github.com/gravitational/version) $(PLANET_GO_LDFLAGS)")
