FROM ubuntu:15.04

RUN apt-get update
RUN apt-get install -y curl make git libc6-dev gcc tar gzip
RUN mkdir -p /opt && cd /opt && curl https://storage.googleapis.com/golang/go1.4.2.linux-amd64.tar.gz | tar xz

ENV GOPATH /gopath
ENV GOROOT /opt/go
ENV PATH $PATH:$GOPATH/bin:$GOROOT/bin

RUN mkdir -p $GOPATH/src $GOPATH/bin

RUN go get github.com/klizhentas/deb2aci github.com/appc/spec/actool github.com/kr/godep
RUN go install github.com/klizhentas/deb2aci github.com/appc/spec/actool

ENV BUILDDIR /build
RUN mkdir -p $BUILDDIR

COPY ./aci-manifest $BUILDDIR/aci-manifest

RUN deb2aci -pkg systemd\
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
            -manifest $BUILDDIR/aci-manifest\
			-image $BUILDDIR/ubuntu.aci


RUN cd $BUILDDIR && tar -xzf ubuntu.aci
ENV ROOTFS $BUILDDIR/rootfs

RUN rm -rf $ROOTFS/lib/systemd/system-generators/systemd-getty-generator \
	 $ROOTFS/lib/systemd/system/sysinit.target.wants/udev-finish.service \
	 $ROOTFS/lib/systemd/system/sysinit.target.wants/systemd-udevd.service \
	 $ROOTFS/lib/systemd/system/getty.target.wants/getty-static.service \
	 $ROOTFS/lib/systemd/system/sysinit.target.wants/debian-fixup.service \
	 $ROOTFS/lib/systemd/system/sysinit.target.wants/sys-kernel-config.mount \
	 $ROOTFS/lib/systemd/system/sysinit.target.wants/systemd-ask-password-console.path \
	 $ROOTFS/lib/systemd/system/sysinit.target.wants/systemd-hwdb-update.service \
	 $ROOTFS/lib/systemd/system/sysinit.target.wants/systemd-binfmt.service \
	 $ROOTFS/lib/systemd/system/sysinit.target.wants/sys-kernel-config.mount \
	 $ROOTFS/lib/systemd/system/sysinit.target.wants/sys-kernel-debug.mount \
	 $ROOTFS/lib/systemd/system/sysinit.target.wants/systemd-ask-password-console.path \
	 $ROOTFS/lib/systemd/system/sysinit.target.wants/systemd-modules-load.service \
	 $ROOTFS/lib/systemd/system/multiuser.target.wants/systemd-logind.service \
	 $ROOTFS/lib/systemd/system/multiuser.target.wants/systemd-ask-password-wall.path \
	 $ROOTFS/lib/systemd/system/multiuser.target.wants/systemd-user-sessions.service \
	 $ROOTFS/lib/systemd/system/sockets.target.wants/docker.socket \
	 $ROOTFS/lib/systemd/system/systemd-poweroff.service \
	 $ROOTFS/lib/systemd/system/systemd-reboot.service \
	 $ROOTFS/lib/systemd/system/systemd-kexec.service

RUN mkdir -p $ROOTFS/run/systemd && ls -l $ROOTFS/run
RUN echo "libcontainer" >  $ROOTFS/run/systemd/container

ADD . $GOPATH/src/github.com/gravitational/cube
RUN go install github.com/gravitational/cube/cube