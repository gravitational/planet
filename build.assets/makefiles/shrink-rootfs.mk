# This makefile runs right after an image was created
# Its purpose is to remove as much garbage out of $(ROOTFS) as possible
all:
	@echo -e "\\n---> Shrinking Planet image in ($(ROOTFS))...\\n"
	rm -rf $(ROOTFS)/usr/share/man
	rm -rf $(ROOTFS)/usr/share/doc
	rm -rf $(ROOTFS)/var/lib/apt
	rm -rf $(ROOTFS)/var/lib/dpkg
	rm -rf $(ROOTFS)/var/log/*
	rm -rf $(ROOTFS)/var/cache
# not sure if this is a good idea... to kill all locales:
	rm -rf $(ROOTFS)/usr/share/locale
