# This makefile runs right after an image was created
# Its purpose is to remove as much garbage out of $(ROOTFS) as possible

# Units to disable:
#
# Disable cgproxy as there might not be a cgmanager running on host
# Disable cgmanager as it only runs outside of container
#
# Disable apt upgrade services
#
# Disable lvm2 metadata daemon/socket and monitor
# Disable block availability service (blk-availability.service) to avoid deactivation
# of block devices on container stop
units := \
	cgproxy.service cgmanager.service \
	apt-daily.service apt-daily-upgrade.service \
	lvm2-monitor.service lvm2-lvmetad.service lvm2-lvmetad.socket \
	blk-availability.service
# disable apt upgrade timers
timers := apt-daily apt-daily-upgrade

all:
	@echo "\n---> Shrinking Planet image in ($(ROOTFS))...\n"
	rm -rf $(ROOTFS)/usr/share/man
	rm -rf $(ROOTFS)/usr/share/doc
	rm -rf $(ROOTFS)/var/lib/apt
	rm -rf $(ROOTFS)/var/log/*
	rm -rf $(ROOTFS)/var/cache
	rm -rf $(ROOTFS)/lib/systemd/system/sysinit.target.wants/proc-sys-fs-binfmt_misc.automount
	$(foreach unit,$(units),rm -f $(ROOTFS)/lib/systemd/system/multi-user.target.wants/$(unit);)
	$(foreach unit,$(units),rm -f $(ROOTFS)/etc/systemd/system/multi-user.target.wants/$(unit);)
	$(foreach unit,$(units),rm -f $(ROOTFS)/etc/systemd/system/sysinit.target.wants/$(unit);)
	$(foreach timer,$(timers),rm -f $(ROOTFS)/lib/systemd/system/timers.target.wants/$(timer).timer;)
	$(foreach timer,$(timers),rm -f $(ROOTFS)/etc/systemd/system/timers.target.wants/$(timer).timer;)
	rm -rf $(ROOTFS)/usr/share/locale
	cd $(ROOTFS) && find -P . -maxdepth 1 -xdev '!' -type l \
		-not -path './proc' \
		-not -path './sys' \
		-not -path './dev' \
		-not -path './.relabelpaths' \
		-not -path '.' \
		-printf '/%P\0' > $(ROOTFS)/.relabelpaths
