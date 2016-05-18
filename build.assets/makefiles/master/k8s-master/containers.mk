.PHONY: all install

SRCDIR:=$(ASSETS)/makefiles/master/k8s-master/containers/
export OUTDIR:=$(ROOTFS)/etc/docker/offline
TARBALLS:=$(OUTDIR)/pause.tar.gz \
		$(OUTDIR)/nettest.tar.gz \
		$(OUTDIR)/hook.tar.gz

all: install $(TARBALLS)

# build container image tarballs
$(OUTDIR)/%.tar.gz: $(SRCDIR)/%.mk
	$(MAKE) -C $(SRCDIR) -f $<

install:
	mkdir -p $(OUTDIR)
	cp -af offline-container-import.service $(ROOTFS)/lib/systemd/system/
	ln -sf /lib/systemd/system/offline-container-import.service $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
