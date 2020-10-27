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
all:
	@echo "\n---> Shrinking Planet image in ($(ROOTFS))...\n"
	rm -rf $(ROOTFS)/usr/share/man
	rm -rf $(ROOTFS)/usr/share/doc
	rm -rf $(ROOTFS)/var/lib/apt
	rm -rf $(ROOTFS)/var/log/*
	rm -rf $(ROOTFS)/var/cache
	rm -rf $(ROOTFS)/lib/systemd/system/sysinit.target.wants/proc-sys-fs-binfmt_misc.automount
	rm -rf $(ROOTFS)/usr/share/locale
