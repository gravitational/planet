.PHONY: all

IMAGE := $(BUILDDIR)/ubuntu.aci

all: $(IMAGE)

$(IMAGE): image.mk
	go get github.com/klizhentas/deb2aci github.com/appc/spec/actool
	go install github.com/klizhentas/deb2aci github.com/appc/spec/actool
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
            -pkg docker.io\
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
