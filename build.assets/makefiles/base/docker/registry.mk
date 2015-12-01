.PHONY: all

REPODIR=$(GOPATH)/src/github.com/docker/
VER=v2.2.0
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

BINARIES=$(TARGETDIR)/registry
GO_LDFLAGS=-ldflags "-X `go list ./version`.Version=$(VER) -w"

all: $(BINARIES)

$(BINARIES):
	@echo "\n---> Building docker registry\n"
	mkdir -p $(REPODIR)
	cd $(REPODIR) && git clone https://github.com/docker/distribution -b $(VER) --depth 1
	cd $(REPODIR)/distribution && \
	echo "$$VERSION_PACKAGE" > version/version.go && \
	GOPATH=$(GOPATH):$(GOPATH)/src/github.com/docker/distribution/Godeps/_workspace GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags "$(DOCKER_BUILDTAGS)" -a -installsuffix cgo -o $@ $(GO_LDFLAGS) $(GO_GCFLAGS) ./cmd/registry
