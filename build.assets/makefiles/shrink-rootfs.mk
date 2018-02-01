# This makefile runs right after an image was created
# Its purpose is to remove as much garbage out of $(ROOTFS) as possible

# Units to disable
# disable cgproxy as there might not be a cgmanager running on host
# disable cgmanager as it only runs outside of container
# disable apt upgrade services
# disable lvm2 metadata daemon/socket and monitor
units := \
	cgproxy.service cgmanager.service \
	apt-daily.service apt-daily-upgrade.service \
	lvm2-monitor.service lvm2-lvmetad.service lvm2-lvmetad.socket
# disable apt upgrade timers
timers := apt-daily apt-daily-upgrade

all:
	@echo "\n---> Shrinking Planet image in ($(ROOTFS))...\n"
	rm -rf $(ROOTFS)/usr/share/man
	rm -rf $(ROOTFS)/usr/share/doc
	rm -rf $(ROOTFS)/var/lib/apt
	rm -rf $(ROOTFS)/var/lib/dpkg
	rm -rf $(ROOTFS)/var/log/*
	rm -rf $(ROOTFS)/var/cache
	rm -rf $(ROOTFS)/lib/systemd/system/sysinit.target.wants/proc-sys-fs-binfmt_misc.automount
	$(foreach unit,$(units),rm -f $(ROOTFS)/lib/systemd/system/multi-user.target.wants/$(unit);)
	$(foreach unit,$(units),rm -f $(ROOTFS)/etc/systemd/system/multi-user.target.wants/$(unit);)
	$(foreach unit,$(units),rm -f $(ROOTFS)/etc/systemd/system/sysinit.target.wants/$(unit);)
	$(foreach timer,$(timers),rm -f $(ROOTFS)/lib/systemd/system/timers.target.wants/$(timer).timer;)
	$(foreach timer,$(timers),rm -f $(ROOTFS)/etc/systemd/system/timers.target.wants/$(timer).timer;)
	rm -rf $(ROOTFS)/usr/share/locale
