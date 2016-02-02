
CONTAINERDIR:=$(ASSETDIR)/k8s-$(KUBE_VER)/containers
CONTAINERMKDIR:=$(ASSETS)/makefiles/kubernetes/containers/

CONTAINERS:=$(CONTAINERDIR)/pause.tar.gz \
		$(CONTAINERDIR)/nettest.tar.gz
# FIXME: this can be a single target with (CONTAINERS) instead of two
ROOTFS_CONTAINERS:=$(ROOTFS)/etc/docker/tmp/pause.tar.gz \
		$(ROOTFS)/etc/docker/tmp/nettest.tar.gz

.PHONY: all install-service

all: install-service $(CONTAINERS) $(ROOTFS_CONTAINERS) 


$(CONTAINERDIR)/%.tar.gz: $(CONTAINERMKDIR)/%.mk
	@echo "Building containers: $(CONTAINERS) $(CONTAINERDIR) $(CONTAINERMKDIR)"
	$(MAKE) -C $(CONTAINERDIR) -f $<

$(ROOTFS)/etc/docker/tmp/%.tar.gz: $(CONTAINERDIR)/%.tar.gz
	cp $< $@

install-service:
	mkdir -p $(ROOTFS)/etc/docker/tmp
	cp -af $(CONTAINERMKDIR)/preloaded-images.service $(ROOTFS)/lib/systemd/system/
	ln -sf /lib/systemd/system/preloaded-images.service $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
