
.PHONY: all
all:
	mkdir -p $(ROOTFS)/lib/systemd/system/systemd-journald.service.d/
	cp -af ./journald.conf $(ROOTFS)/lib/systemd/system/systemd-journald.service.d/
	mkdir -p $(ROOTFS)/etc/systemd/system.conf.d/
	cp -af ./system.conf $(ROOTFS)/etc/systemd/system.conf.d/
