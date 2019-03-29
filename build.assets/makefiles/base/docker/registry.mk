.PHONY: all

REPODIR=$(GOPATH)/src/github.com/docker/
VER=v2.7.1-gravitational

# VERSION_PACKAGE defines contents of a `version.go` file which is part of docker
# registry source code distribution responsible for defining a registry's Version
# string. The Version is set to the branch we use to pull/build registry executable.
define VERSION_PACKAGE
package version

// Package is the overall, canonical project import path under which the
// package was built.
var Package = "planet/docker/distribution"

// Version indicates which version of the binary is running. This is set to
// the latest release tag by hand, always suffixed by "+unknown". During
// build, it will be replaced by the actual version. The value here will be
// used if the registry is run after a go get based install.
var Version = "$(VER)"
endef
export VERSION_PACKAGE

BINARIES:=$(ASSETDIR)/registry-$(VER)
GO_LDFLAGS=-ldflags "-X `go list ./version`.Version=$(VER) -w"


all: $(BINARIES) install

$(BINARIES):
	@echo "\n---> Building docker registry:\n"
	mkdir -p $(REPODIR)
#	cd $(REPODIR) && git clone https://github.com/docker/distribution -b $(VER) --depth 1
	cd $(REPODIR) && git clone https://github.com/gravitational/distribution -b $(VER) --depth 1
	cd $(REPODIR)/distribution && \
	echo "$$VERSION_PACKAGE" > version/version.go && \
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags "$(DOCKER_BUILDTAGS)" -a -installsuffix cgo -o $@ $(GO_LDFLAGS) ./cmd/registry

install: registry.mk $(BINARIES)
	@echo "\n---> Installing docker registry:\n"
	cp -af $(ASSETS)/makefiles/base/docker/registry.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/registry.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
	mkdir -p $(ROOTFS)/usr/bin
	cp $(BINARIES) $(ROOTFS)/usr/bin/registry
	mkdir -p $(ROOTFS)/etc/docker/registry
	cp $(ASSETS)/docker/registry/config.yml $(ROOTFS)/etc/docker/registry/
