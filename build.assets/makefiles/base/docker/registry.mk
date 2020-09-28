.PHONY: all

PLANET_PKG_PATH := /gopath/src/github.com/gravitational/planet
VER_PKG_PATH := github.com/docker/distribution/version
PKG_PATH := github.com/docker/distribution
VER ?= v2.7.2+gravitational
BINARIES ?= $(ASSETDIR)/registry-$(VER)
GO_LDFLAGS ?= -ldflags "-X $(VER_PKG_PATH).Version=$(VER) -X $(VER_PKG_PATH).Package=$(PKG_PATH) -w"


all: $(BINARIES) install

$(BINARIES):
	@echo "\n---> Building docker registry:\n"
	cd $(PLANET_PKG_PATH) && \
	GOOS=linux GOARCH=amd64 GO111MODULE=on \
		go build -mod=vendor -tags "$(DOCKER_BUILDTAGS)" -a -installsuffix cgo -o $@ $(GO_LDFLAGS) github.com/gravitational/planet/tool/registry/...

install: registry.mk $(BINARIES)
	@echo "\n---> Installing docker registry:\n"
	cp -af $(ASSETS)/makefiles/base/docker/registry.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/registry.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
	mkdir -p $(ROOTFS)/usr/bin
	cp $(BINARIES) $(ROOTFS)/usr/bin/registry
	mkdir -p $(ROOTFS)/etc/docker/registry
	cp $(ASSETS)/docker/registry/config.yml $(ROOTFS)/etc/docker/registry/
