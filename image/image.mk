.PHONY: all

IMAGE := $(BUILDDIR)/ubuntu.aci

all: image.mk $(IMAGE)

$(IMAGE):
	go install github.com/klizhentas/deb2aci
	deb2aci -pkg systemd\
            -pkg dbus\
            -pkg liblzma5\
            -pkg bash\
            -pkg iptables\
            -pkg coreutils\
            -pkg grep\
            -pkg findutils\
            -pkg binutils\
            -pkg less\
            -manifest ./aci-manifest\
			-image $(IMAGE)
	cd $(BUILDDIR) && tar -xzf ubuntu.aci
	cp -a ./rootfs/. $(BUILDDIR)/rootfs
	rm $(BUILDDIR)/rootfs/lib/systemd/system/sysinit.target.wants/udev-finish.service
	rm $(BUILDDIR)/rootfs/lib/systemd/system/sysinit.target.wants/debian-fixup.service
