.PHONY: all install-service

SRCDIR:=$(ASSETS)/makefiles/kubernetes/containers/
export OUTDIR:=$(ROOTFS)/etc/docker/offline
TARBALLS:=$(OUTDIR)/pause.tar.gz \
		$(OUTDIR)/nettest.tar.gz

all: install-service $(TARBALLS)

# build all container image tarballs
$(OUTDIR)/%.tar.gz: $(SRCDIR)/%.mk
	$(MAKE) -C $(SRCDIR) -f $<

install-service:
	mkdir -p $(OUTDIR)
	cp -af $(SRCDIR)/preloaded-images.service $(ROOTFS)/lib/systemd/system/
	ln -sf /lib/systemd/system/preloaded-images.service $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
