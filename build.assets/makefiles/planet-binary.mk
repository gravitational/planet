# This makefile runs inside the docker buildbox
# The following volumes are mounted and shared with the host:
ASSETS := /assets
ROOTFS := /rootfs
TARGETDIR := /targetdir
ASSETDIR := /assetdir
LINKFLAGS_TAG := master
PLANET_PKG_PATH := /gopath/src/github.com/gravitational/planet

.PHONY: all
all: $(ASSETDIR)/planet $(ASSETDIR)/docker-import

$(ASSETDIR)/planet: flags
# Add to ldflags to compile a completely static version of the planet binary (w/o the glibc dependency)
# -ldflags '-extldflags "-static"'
	GOOS=linux GOARCH=amd64 go build -ldflags $(PLANET_LINKFLAGS) -o $@ github.com/gravitational/planet/tool/planet

$(ASSETDIR)/docker-import:
	GOOS=linux GOARCH=amd64 go build -ldflags "$(PLANET_GO_LDFLAGS)" -o $@ github.com/gravitational/planet/tool/docker-import

.PHONY: flags
flags:
	go install github.com/gravitational/version/cmd/linkflags
	$(eval PLANET_LINKFLAGS := "$(shell linkflags -pkg=$(PLANET_PKG_PATH) -verpkg=github.com/gravitational/planet/vendor/github.com/gravitational/version) $(PLANET_GO_LDFLAGS)")
