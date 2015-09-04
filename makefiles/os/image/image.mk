.PHONY: all

IMAGE := $(BUILDDIR)/ubuntu.aci

all: $(IMAGE)

$(IMAGE): image.mk
	deb2aci -pkg systemd\
            -pkg dbus\
            -pkg liblzma5\
            -pkg bash\
            -pkg iptables\
            -pkg coreutils\
            -pkg grep\
            -pkg findutils\
            -pkg binutils\
            -pkg net-tools\
            -pkg less\
            -pkg iproute2\
            -pkg bridge-utils\
            -pkg kmod\
            -pkg openssl\
            -pkg adduser\
            -pkg iptables\
            -pkg init-system-helpers\
            -pkg lsb-base\
            -pkg perl\
            -pkg libc6\
            -pkg libdevmapper1.02.1\
            -pkg libsqlite3-0\
            -pkg git\
            -pkg aufs-tools\
            -pkg xz-utils\
            -pkg gawk\
            -pkg dash\
            -pkg iproute2\
            -pkg ca-certificates\
			-pkg aufs-tools\
            -pkg sed\
            -pkg curl\
            -pkg e2fsprogs\
			-pkg libncurses5\
            -pkg ncurses-base\
            -manifest ./aci-manifest\
			-image $(IMAGE)
	cd $(BUILDDIR) && tar -xzf ubuntu.aci && mv ./rootfs $(ROOTFS)
	cp -a ./rootfs/. $(ROOTFS)

	rm -rf $(ROOTFS)/lib/systemd/system-generators/systemd-getty-generator

	rm -f $(ROOTFS)/lib/systemd/system/sysinit.target.wants/udev-finish.service
	rm -f $(ROOTFS)/lib/systemd/system/sysinit.target.wants/systemd-udevd.service
	rm -f $(ROOTFS)/lib/systemd/system/getty.target.wants/getty-static.service

	rm -f $(ROOTFS)/lib/systemd/system/sysinit.target.wants/debian-fixup.service
	rm -f $(ROOTFS)/lib/systemd/system/sysinit.target.wants/sys-kernel-config.mount
	rm -f $(ROOTFS)/lib/systemd/system/sysinit.target.wants/systemd-ask-password-console.path
	rm -f $(ROOTFS)/lib/systemd/system/sysinit.target.wants/systemd-hwdb-update.service
	rm -f $(ROOTFS)/lib/systemd/system/sysinit.target.wants/systemd-binfmt.service
	rm -f $(ROOTFS)/lib/systemd/system/sysinit.target.wants/sys-kernel-config.mount
	rm -f $(ROOTFS)/lib/systemd/system/sysinit.target.wants/sys-kernel-debug.mount
	rm -f $(ROOTFS)/lib/systemd/system/sysinit.target.wants/systemd-ask-password-console.path
	rm -f $(ROOTFS)/lib/systemd/system/sysinit.target.wants/systemd-modules-load.service

	rm -f $(ROOTFS)/lib/systemd/system/multiuser.target.wants/systemd-logind.service
	rm -f $(ROOTFS)/lib/systemd/system/multiuser.target.wants/systemd-ask-password-wall.path
	rm -f $(ROOTFS)/lib/systemd/system/multiuser.target.wants/systemd-user-sessions.service

	rm -f $(ROOTFS)/lib/systemd/system/sockets.target.wants/docker.socket

	rm -f $(ROOTFS)/lib/systemd/system/systemd-poweroff.service
	rm -f $(ROOTFS)/lib/systemd/system/systemd-reboot.service
	rm -f $(ROOTFS)/lib/systemd/system/systemd-kexec.service

# tell systemd it runs in virt mode
	mkdir -p $(ROOTFS)/run/systemd/
	echo "libcontainer" >  $(ROOTFS)/run/systemd/container

# make sure systemd is owned by root
	chown root:root -R $(ROOTFS)/bin/
