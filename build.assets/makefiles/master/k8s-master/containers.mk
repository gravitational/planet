.PHONY: all install

SRCDIR:=$(ASSETS)/makefiles/master/k8s-master/containers/
export OUTDIR:=$(ROOTFS)/etc/docker/offline
TARBALLS:=$(OUTDIR)/pause.tar.gz \
		$(OUTDIR)/nettest.tar.gz

all: $(ASSETDIR)/docker-bulkimport install $(TARBALLS)

# build container image tarballs
$(OUTDIR)/%.tar.gz: $(SRCDIR)/%.mk
	$(MAKE) -C $(SRCDIR) -f $<

install:
	mkdir -p $(OUTDIR)
	cp -af offline-container-import.service $(ROOTFS)/lib/systemd/system/
	ln -sf /lib/systemd/system/offline-container-import.service $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
	cp $(ASSETDIR)/docker-bulkimport $(ROOTFS)/usr/bin/

$(ASSETDIR)/docker-bulkimport:
	GOOS=linux GOARCH=amd64 go build -ldflags "$(PLANET_GO_LDFLAGS)" -o $@ github.com/gravitational/planet/tool/docker
