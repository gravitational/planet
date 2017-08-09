# This makefile runs right after an image was created
# Its purpose is to remove as much garbage out of $(ROOTFS) as possible
all:
	@echo "\n---> Shrinking Planet image in ($(ROOTFS))...\n"
	rm -rf $(ROOTFS)/usr/share/man
	rm -rf $(ROOTFS)/usr/share/doc
	rm -rf $(ROOTFS)/var/lib/apt
	rm -rf $(ROOTFS)/var/lib/dpkg
	rm -rf $(ROOTFS)/var/log/*
	rm -rf $(ROOTFS)/var/cache
	rm -rf $(ROOTFS)/lib/systemd/system/sysinit.target.wants/proc-sys-fs-binfmt_misc.automount
	# disable cgproxy - the host might not running cgmanager in the first place
	rm $(ROOTFS)/lib/systemd/system/cgproxy.service
	# not sure if this is a good idea... to kill all locales:
	rm -rf $(ROOTFS)/usr/share/locale
