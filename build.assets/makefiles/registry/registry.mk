.PHONY: all

# TODO: build registry
VER := 2.2.0

all: registry.mk
	@echo "\\n---> Creating registry service\\n"
	cp -af ./registry.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/registry.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
	mkdir -p $(ROOTFS)/usr/bin
	cp $(ASSETS)/docker/registry/registry $(ROOTFS)/usr/bin/
	mkdir -p $(ROOTFS)/etc/docker/registry
	cp $(ASSETS)/docker/registry/config.yml $(ROOTFS)/etc/docker/registry/
