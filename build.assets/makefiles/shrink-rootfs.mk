# This makefile runs right after an image was created
# Its purpose is to remove as much garbage out of $(ROOTFS) as possible

# Services to disable
# remove cgproxy as there might not be a cgmanager running on host
# remove cgmanager as it only runs outside of container
# remove apt upgrade services
services := cgproxy cgmanager apt-daily apt-daily-upgrade
# remove apt upgrade timers
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
	$(foreach service,$(services),rm -f $(ROOTFS)/lib/systemd/system/multi-user.target.wants/$(service).service;)
	$(foreach service,$(services),rm -f $(ROOTFS)/etc/systemd/system/multi-user.target.wants/$(service).service;)
	$(foreach timer,$(timers),rm -f $(ROOTFS)/lib/systemd/system/timers.target.wants/$(timer).timer;)
	$(foreach timer,$(timers),rm -f $(ROOTFS)/etc/systemd/system/timers.target.wants/$(timer).timer;)
	# not sure if this is a good idea... to kill all locales:
	rm -rf $(ROOTFS)/usr/share/locale
