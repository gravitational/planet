# syntax = docker/dockerfile:1.2

ARG KUBE_VER=v1.21.2
ARG SECCOMP_VER=2.3.1-2.1+deb9u1
ARG DOCKER_VER=20.10.7
# we currently use our own flannel fork: gravitational/flannel
ARG FLANNEL_VER=v0.10.5-gravitational
ARG HELM_VER=2.16.12
ARG HELM3_VER=3.3.4
ARG COREDNS_VER=1.7.0
ARG NODE_PROBLEM_DETECTOR_VER=v0.6.4
ARG CNI_VER=0.8.6
ARG IPTABLES_VER=v1.8.5
ARG PLANET_UID=980665
ARG PLANET_GID=980665
ARG GO_VERSION=1.17.5
ARG ALPINE_VERSION=3.12
ARG DEBIAN_IMAGE=quay.io/gravitational/debian-mirror@sha256:4b6ec644c29e4964a6f74543a5bf8c12bed6dec3d479e039936e4a37a8af9116
ARG GO_BUILDER_VERSION=go1.17.5-stretch
ARG AWS_ENCRYPTION_PROVIDER_VER=c4abcb30b4c1ab1961369e1e50a98da2cedb765d
# TODO(dima): update to 2.7.2 release once available
# ARG DISTRIBUTION_VER=release/2.7
ARG DISTRIBUTION_VER=v2.7.1-gravitational
ARG ARTEFACTS_DIR=_build

ARG PLANET_PKG_PATH=/gopath/src/github.com/gravitational/planet
ARG PLANET_BUILDFLAGS="-tags 'selinux sqlite_omit_load_extension'"

# ETCD Versions to include in the release
# This list needs to include every version of etcd that we can upgrade from + latest
# Version log
# v3.3.4
# v3.3.9  - 5.2.x,
# v3.3.11 - 5.5.x,
# v3.3.12 - 6.3.x, 6.1.x, 5.5.x
# v3.3.15 - 6.3.x
# v3.3.20 - 6.3.x, 6.1.x, 5.5.x
# v3.3.22 - 6.3.x, 6.1.x, 5.5.x
# v3.4.3  - 7.0.x
# v3.4.7  - 7.0.x
# v3.4.9  - 7.0.x
ARG ETCD_VER="v3.3.12 v3.3.15 v3.3.20 v3.3.22 v3.4.3 v3.4.7 v3.4.9"
ARG ETCD_LATEST_VER=v3.4.9

# go builder
FROM golang:${GO_VERSION}-stretch AS gobase
RUN apt install -y --no-install-recommends git

FROM alpine:${ALPINE_VERSION} AS downloader
RUN apk add --no-cache curl tar && mkdir -p /tmp

FROM ${DEBIAN_IMAGE} AS iptables-builder
ARG IPTABLES_VER
RUN --mount=type=cache,sharing=locked,target=/var/cache/apt --mount=type=cache,sharing=locked,target=/var/lib/apt \
	set -ex && \
	apt-get update && apt-get install -y --no-install-recommends \
		git pkg-config autoconf automake libtool libmnl-dev make build-essential
RUN set -ex && \
        mkdir /tmp/iptables.build /tmp/iptables.local && \
        git clone git://git.netfilter.org/iptables.git --branch ${IPTABLES_VER} --single-branch /tmp/iptables.build && \
        cd /tmp/iptables.build && \
        ./autogen.sh && \
        ./configure --disable-nftables --prefix=/usr/local && \
        make && \
        make install

# Builder box
# FIXME(dima): for Go1.16 use:
# go install github.com/gravitational/version/cmd/linkflags@latest
ARG GO_BUILDER_VERSION
FROM quay.io/gravitational/debian-venti:${GO_BUILDER_VERSION} AS planet-builder-base
RUN apt-get update && apt-get install -y libc6-dev libudev-dev && mkdir -p /tmp && \
	GO111MODULE=on go install github.com/gravitational/version/cmd/linkflags@0.0.2

FROM planet-builder-base AS planet-builder
ENV PATH="$PATH:/gopath/bin"
WORKDIR /gopath/src/github.com/gravitational/planet
RUN --mount=target=. --mount=target=/root/.cache,type=cache --mount=target=/go/pkg/mod,type=cache \
	set -ex && \
	CGO_LDFLAGS_ALLOW=".*" \
	GOOS=linux GOARCH=amd64 GO111MODULE=on \
	go build -mod=vendor -ldflags "$(linkflags -pkg=. -verpkg=github.com/gravitational/version)" -tags "selinux sqlite_omit_load_extension" -o /planet ./tool/planet/...

FROM planet-builder-base AS docker-import-builder
WORKDIR /gopath/src/github.com/gravitational/planet
RUN --mount=target=. --mount=target=/root/.cache,type=cache --mount=target=/go/pkg/mod,type=cache \
	set -ex && \
	GOOS=linux GOARCH=amd64 \
	go build -mod=vendor -o /docker-import github.com/gravitational/planet/tool/docker-import

FROM gobase AS create-tarball-builder
WORKDIR /go/src/github.com/gravitational/planet
RUN --mount=target=. --mount=target=/root/.cache,type=cache --mount=target=/go/pkg/mod,type=cache \
	set -ex && \
	GOOS=linux GOARCH=amd64 GCO_ENABLED=0 \
	go build -mod=vendor -o /create-tarball ./tool/create-tarball/...

# OS base image
# debian:stretch-backports tagged 20200501
FROM ${DEBIAN_IMAGE} AS os
ARG SECCOMP_VER
# planet user to use inside the rootfs tarball. This serves as a placeholder
# and the files will be owned by the actual planet user after extraction
ARG PLANET_UID
ARG PLANET_GID

ENV DEBIAN_FRONTEND noninteractive

COPY ./build.assets/docker/os-rootfs/ /

RUN --mount=type=cache,sharing=locked,target=/var/cache/apt --mount=type=cache,sharing=locked,target=/var/lib/apt \
	set -ex; \
	if ! command -v gpg > /dev/null; then \
		apt-get update; \
		apt-get install -y --no-install-recommends \
		gnupg2 \
		dirmngr; \
	fi

RUN --mount=type=cache,target=/var/cache/apt,rw --mount=type=cache,target=/var/lib/apt,rw \
	set -ex && \
	sed -i 's/main/main contrib non-free/g' /etc/apt/sources.list && \
	apt-get update && apt-get -q -y install apt-transport-https \
	&& apt-get install -q -y apt-utils less locales \
	&& apt-get install -t stretch-backports -q -y systemd

# Set locale to en_US.UTF-8
RUN locale-gen \
	&& locale-gen en_US.UTF-8 \
	&& dpkg-reconfigure locales

# https://github.com/systemd/systemd/blob/v230/src/shared/install.c#L413
# Exit code 1 is either created successfully or symlink already exists
# Exit codes < 0 are failures
RUN systemctl set-default multi-user.target; if [ "$?" -lt 0 ]; then exit $?; fi;
RUN set -ex && systemctl mask \
	cgproxy.service cgmanager.service \
	apt-daily.service apt-daily-upgrade.service \
	lvm2-monitor.service lvm2-lvmetad.service lvm2-lvmetad.socket \
	blk-availability.service \
	# Mask timers
	apt-daily.timer apt-daily-upgrade.timer \
	# Mask mount units and getty service so that we don't get login prompt
	systemd-remount-fs.service dev-hugepages.mount sys-fs-fuse-connections.mount \
	getty.target console-getty.service;

# TODO(r0mant): Disable *iscsi* services since they may be running on host
#               In the future we will need to enable them conditionally to
#               be able to support OpenEBS cStor engine out of the box
RUN set -ex && \
	systemctl mask iscsi.service iscsid.service open-iscsi.service systemd-udevd.service

ENV LANGUAGE en_US.UTF-8
ENV LANG en_US.UTF-8
ENV LC_ALL en_US.UTF-8
ENV LC_CTYPE en_US.UTF-8
ENV PAGER /usr/bin/less
ENV LESS -isM

# Base planet image
FROM os AS base
ARG SECCOMP_VER
ARG PLANET_UID
ARG PLANET_GID

ENV DEBIAN_FRONTEND noninteractive

COPY --from=iptables-builder /usr/local/ /usr/local/

# FIXME: allowing downgrades and pinning the version of libip4tc0 for iptables
# as the package has a dependency on the older version as the one available.
RUN --mount=type=cache,target=/var/cache/apt,rw --mount=type=cache,target=/var/lib/apt,rw \
	set -ex && \
	apt-get update && \
	apt-get install -q -y --allow-downgrades --no-install-recommends \
	bridge-utils \
	seccomp=$SECCOMP_VER \
	bash-completion \
	kmod \
	libip4tc0=1.6.0+snapshot20161117-6 \
	ebtables \
	libsqlite3-0 \
	e2fsprogs \
	libncurses5 \
	net-tools \
	curl \
	iproute2 \
	lsb-base \
	dash \
	ca-certificates \
	aufs-tools \
	xfsprogs \
	dbus \
	dnsutils \
	ethtool \
	sysstat \
	nano \
	vim \
	iotop \
	htop \
	ifstat \
	iftop \
	traceroute \
	tcpdump \
	procps \
	coreutils \
	lsof \
	socat \
	nmap \
	netcat \
	nfs-common \
	jq \
	conntrack \
	open-iscsi \
	strace \
	netbase \
	file \
	bsdmainutils; \
	# update loader's cache after pulling in iptables build
	ldconfig

RUN set -ex && \
	groupadd --system --non-unique --gid ${PLANET_GID} planet && \
	useradd --system --non-unique --no-create-home -g ${PLANET_GID} -u ${PLANET_UID} planet && \
	groupadd --system docker && \
	usermod -a -G planet root && \
	usermod -a -G docker planet;

FROM gobase AS flannel-builder
ARG FLANNEL_VER
RUN --mount=target=/root/.cache,type=cache --mount=target=/go/pkg/mod,type=cache \
	set -ex && \
	mkdir -p /go/src/github.com/coreos && \
	cd /go/src/github.com/coreos && \
	git clone https://github.com/gravitational/flannel -b ${FLANNEL_VER} --depth 1 && \
	cd /go/src/github.com/coreos/flannel && \
	go build -mod=vendor -o /flanneld .

FROM gobase AS aws-encryption-builder
ARG AWS_ENCRYPTION_PROVIDER_VER
RUN --mount=target=/root/.cache,type=cache --mount=target=/go/pkg/mod,type=cache \
	set -ex && \
	mkdir -p /go/src/github.com/kubernetes-sigs && \
	cd /go/src/github.com/kubernetes-sigs && \
	git clone https://github.com/kubernetes-sigs/aws-encryption-provider && \
	cd /go/src/github.com/kubernetes-sigs/aws-encryption-provider && \
	git checkout ${AWS_ENCRYPTION_PROVIDER_VER} && \
	make build-server && \
	cp /go/src/github.com/kubernetes-sigs/aws-encryption-provider/bin/aws-encryption-provider /aws-encryption-provider

FROM gobase AS distribution-builder
ARG DISTRIBUTION_VER
ENV GOOS=linux GOARCH=amd64 CGO_ENABLED=0 GO111MODULE=off 
WORKDIR /go/src/github.com/docker/distribution
RUN --mount=target=/root/.cache,type=cache --mount=target=/go/pkg/mod,type=cache \
	set -ex && \
	git clone https://github.com/gravitational/distribution -b ${DISTRIBUTION_VER} --depth 1 . && \
	go build -a -installsuffix cgo -o /registry -ldflags "-X `go list ./version`.Version=${DISTRIBUTION_VER} -X `go list ./version`.Package=planet/docker/distribution -w" ./cmd/registry

FROM downloader AS cni-downloader
ARG CNI_VER
RUN set -ex && \
	mkdir -p /opt/cni/bin && \
	curl -L --retry 5 \
	https://github.com/containernetworking/plugins/releases/download/v${CNI_VER}/cni-plugins-linux-amd64-v${CNI_VER}.tgz -o /tmp/cni.tar.gz && \
	tar -xzvf /tmp/cni.tar.gz --no-same-owner -C /opt/cni/bin ./bridge ./loopback ./host-local ./portmap ./tuning ./flannel

FROM downloader AS coredns-downloader
ARG COREDNS_VER
RUN set -ex && \
	curl -L --retry 5 https://github.com/coredns/coredns/releases/download/v${COREDNS_VER}/coredns_${COREDNS_VER}_linux_amd64.tgz -o /tmp/coredns.tar.gz && \
	tar xvzf /tmp/coredns.tar.gz --no-same-owner -C / coredns

FROM downloader AS helm-downloader
ARG HELM_VER
RUN set -ex && \
	curl https://get.helm.sh/helm-v${HELM_VER}-linux-amd64.tar.gz -o /tmp/helm.tar.gz && \
	tar xvzf /tmp/helm.tar.gz --no-same-owner --strip-components=1 -C / linux-amd64/helm

FROM downloader AS helm3-downloader
ARG HELM3_VER
RUN set -ex && \
	curl https://get.helm.sh/helm-v${HELM3_VER}-linux-amd64.tar.gz -o /tmp/helm3.tar.gz && \
	tar --transform='flags=r;s|helm|helm3|' -xvzf /tmp/helm3.tar.gz --no-same-owner --strip-components=1 -C / linux-amd64/helm

FROM downloader AS k8s-downloader
ARG KUBE_VER
ENV DOWNLOAD_URL=https://storage.googleapis.com/kubernetes-release/release/${KUBE_VER}/bin/linux/amd64
ENV BINARIES="kube-apiserver kube-controller-manager kube-scheduler kubectl kube-proxy kubelet"
RUN set -ex && \
	for r in ${BINARIES}; do \
		curl --retry 100 --retry-delay 0 --connect-timeout 10 --max-time 300 --tlsv1.2 --silent --show-error -o /tmp/$r ${DOWNLOAD_URL}/$r; \
		chmod +x /tmp/$r; \
	done;

FROM downloader AS etcd-downloader
ARG ETCD_VER
ENV OS=linux
ENV ARCH=amd64
RUN set -ex && \
	mkdir -p /tmp/bin/ && \
	cd /tmp && \
	for v in ${ETCD_VER}; do \
		curl -L https://github.com/etcd-io/etcd/releases/download/$v/etcd-$v-${OS}-${ARCH}.tar.gz -O; \
		tar xf /tmp/etcd-$v-${OS}-${ARCH}.tar.gz \
			--no-same-owner \
			--strip-components 1 \
			--directory /tmp/bin/ \
			--transform="s|etcd$|etcd-$v|" \
			etcd-$v-${OS}-${ARCH}/etcd; \
		tar xf /tmp/etcd-$v-${OS}-${ARCH}.tar.gz \
			--no-same-owner \
			--strip-components 1 \
			--directory /tmp/bin/ \
			--transform="s|etcdctl$|etcdctl-cmd-$v|" \
			etcd-$v-${OS}-${ARCH}/etcdctl; \
	done;

FROM downloader AS node-problem-detector-downloader
ARG NODE_PROBLEM_DETECTOR_VER
RUN set -ex && curl -L --retry 5 https://github.com/kubernetes/node-problem-detector/releases/download/${NODE_PROBLEM_DETECTOR_VER}/node-problem-detector-${NODE_PROBLEM_DETECTOR_VER}.tar.gz| tar xz --no-same-owner -C /tmp/

# docker.mk
FROM downloader AS docker-downloader
ARG DOCKER_VER
RUN set -ex && \
	mkdir -p /docker && \
	curl -L --retry 5 https://download.docker.com/linux/static/stable/x86_64/docker-${DOCKER_VER}.tgz | tar xvz --no-same-owner --strip-components=1 -C /docker

FROM base AS rootfs
ARG ETCD_LATEST_VER
ARG ARTEFACTS_DIR

# systemd.mk
RUN set -ex && \
	mkdir -p \
		/lib/systemd/system/systemd-journald.service.d/ \
		/etc/systemd/system.conf.d/ \
		/etc/docker/offline/
COPY ./build.assets/makefiles/base/systemd/journald.conf /lib/systemd/system/systemd-journald.service.d/
COPY ./build.assets/makefiles/base/systemd/system.conf /etc/systemd/system.conf.d/

# containers.mk
COPY ${ARTEFACTS_DIR}/nettest.tar.gz /etc/docker/offline/
COPY ${ARTEFACTS_DIR}/pause.tar.gz /etc/docker/offline/
COPY ./build.assets/makefiles/master/k8s-master/offline-container-import.service /lib/systemd/system/
RUN set -ex && \
	ln -sf /lib/systemd/system/offline-container-import.service /lib/systemd/system/multi-user.target.wants/

# network.mk
COPY --from=flannel-builder /flanneld /usr/bin/flanneld
COPY ./build.assets/makefiles/base/network/flanneld.service /lib/systemd/system/
# Setup cni and include flannel as a plugin
RUN set -ex && mkdir -p /etc/cni/net.d/ /opt/cni/bin
COPY --from=cni-downloader /opt/cni/bin/ /opt/cni/bin/

# scripts to wait for etcd/flannel to come up
RUN --mount=target=/host \
	set -ex && \
	mkdir -p /usr/bin/scripts && \
	install -m 0755 /host/build.assets/makefiles/base/network/wait-for-etcd.sh /usr/bin/scripts && \
	install -m 0755 /host/build.assets/makefiles/base/network/wait-for-flannel.sh /usr/bin/scripts && \
	install -m 0755 /host/build.assets/makefiles/base/network/setup-etc.sh /usr/bin/scripts

# encryption.mk
COPY --from=aws-encryption-builder /aws-encryption-provider /usr/bin/aws-encryption-provider
COPY ./build.assets/makefiles/encryption/aws-encryption-provider.service /lib/systemd/system/
RUN set -ex && \
	mkdir -p /etc/kmsplugin && \
	chmod o+t /etc/kmsplugin && \
	chmod a+rwx /etc/kmsplugin

# node-problem-detector.mk
COPY --from=node-problem-detector-downloader /tmp/bin/ /usr/bin
COPY ./build.assets/makefiles/base/node-problem-detector/node-problem-detector.service /lib/systemd/system/
RUN set -ex && ln -sf /lib/systemd/system/node-problem-detector.service /lib/systemd/system/multi-user.target.wants/

# dns.mk
RUN set -ex && mkdir -p /etc/coredns/configmaps/ /usr/lib/sysusers.d/
COPY ./build.assets/makefiles/base/dns/coredns.service /lib/systemd/system/
RUN set -ex && ln -sf /lib/systemd/system/coredns.service /lib/systemd/system/multi-user.target.wants/

# docker.mk
COPY ./build.assets/makefiles/base/docker/docker.service /lib/systemd/system/
COPY ./build.assets/makefiles/base/docker/docker.socket /lib/systemd/system/
ENV REGISTRY_ALIASES="apiserver:5000 leader.telekube.local:5000 leader.gravity.local:5000 registry.local:5000"
RUN set -ex && \
	ln -sf /lib/systemd/system/docker.service /lib/systemd/system/multi-user.target.wants/ && \
	for r in ${REGISTRY_ALIASES}; do \
		mkdir -p /etc/docker/certs.d/$r; \
		ln -sf /var/state/root.cert /etc/docker/certs.d/$r/$r.crt; \
		ln -sf /var/state/kubelet.cert /etc/docker/certs.d/$r/client.cert; \
		ln -sf /var/state/kubelet.key /etc/docker/certs.d/$r/client.key; \
	done;
RUN --mount=target=/host \
	set -ex && \
	install -m 0755 /host/build.assets/makefiles/base/docker/unmount-devmapper.sh /usr/bin/scripts/
COPY --from=docker-downloader /docker/ /usr/bin/

# agent.mk
COPY ./build.assets/makefiles/base/agent/planet-agent.service /lib/systemd/system
RUN set -ex && \
	ln -sf /lib/systemd/system/planet-agent.service /lib/systemd/system/multi-user.target.wants/

# kubernetes.mk
COPY --from=k8s-downloader /tmp/ /usr/bin/
COPY --from=helm-downloader /helm /usr/bin/
COPY --from=helm3-downloader /helm3 /usr/bin/
COPY --from=coredns-downloader /coredns /usr/bin/
COPY --from=docker-import-builder /docker-import /usr/bin/
COPY ./build.assets/makefiles/master/k8s-master/*.service /lib/systemd/system/
RUN --mount=target=/host \
	set -ex && \
	cp -TRv -p /host/build.assets/makefiles/master/k8s-master/rootfs/etc/kubernetes /etc/kubernetes && \
	ln -sf /lib/systemd/system/kube-kubelet.service /lib/systemd/system/multi-user.target.wants/ && \
	ln -sf /lib/systemd/system/kube-proxy.service /lib/systemd/system/multi-user.target.wants/ && \
	mkdir -p /usr/bin/scripts && \
	install -m 0755 /host/build.assets/makefiles/master/k8s-master/cluster-dns.sh /usr/bin/scripts/
# etcd.mk
COPY --from=etcd-downloader /tmp/bin/ /usr/bin/
COPY --from=planet-builder /planet /usr/bin/
COPY ./build.assets/makefiles/etcd/etcd.service /lib/systemd/system/
COPY ./build.assets/makefiles/etcd/etcd-upgrade.service /lib/systemd/system/
COPY ./build.assets/makefiles/etcd/etcd-gateway.dropin /lib/systemd/system/
COPY ./build.assets/makefiles/etcd/etcdctl3 /usr/bin/etcdctl3
COPY ./build.assets/makefiles/etcd/etcdctl /usr/bin/etcdctl
RUN set -ex && \
	chmod +x /usr/bin/etcdctl3 /usr/bin/etcdctl && \
	ln -sf /lib/systemd/system/etcd.service /lib/systemd/system/multi-user.target.wants/ && \
	# mask the etcd-upgrade service so that it can only be run if intentionally unmasked
	ln -sf /dev/null /etc/systemd/system/etcd-upgrade.service && \
	# write to the release file to indicate the latest release
	echo PLANET_ETCD_VERSION=${ETCD_LATEST_VER} >> /etc/planet-release

# registry.mk
COPY --from=distribution-builder /registry /usr/bin/registry
COPY ./build.assets/makefiles/base/docker/registry.service /lib/systemd/system
COPY ./build.assets/docker/registry/config.yml /etc/docker/registry/
RUN set -ex && \
	ln -sf /lib/systemd/system/registry.service /lib/systemd/system/multi-user.target.wants/

FROM gobase AS tarball-builder
ARG ETCD_LATEST_VER
ARG KUBE_VER
ARG FLANNEL_VER
ARG DOCKER_VER
ARG HELM_VER
ARG HELM3_VER
ARG COREDNS_VER
ARG NODE_PROBLEM_DETECTOR_VER
ENV REPLACE_ETCD_LATEST_VERSION=${ETCD_LATEST_VER}
ENV REPLACE_KUBE_LATEST_VERSION=${KUBE_VER}
ENV REPLACE_FLANNEL_LATEST_VERSION=${FLANNEL_VER}
ENV REPLACE_DOCKER_LATEST_VERSION=${DOCKER_VER}
ENV REPLACE_HELM_LATEST_VERSION=${HELM_VER}
ENV REPLACE_HELM3_LATEST_VERSION=${HELM3_VER}
ENV REPLACE_COREDNS_LATEST_VERSION=${COREDNS_VER}
ENV REPLACE_NODE_PROBLEM_DETECTOR_LATEST_VERSION=${NODE_PROBLEM_DETECTOR_VER}
COPY ./build.assets/docker/os-rootfs/etc/planet/orbit.manifest.json /output/
RUN --mount=from=rootfs,src=/,target=/output/rootfs \
 --mount=from=create-tarball-builder,src=/create-tarball,target=/create-tarball \
	set -ex && /create-tarball /output /planet.tar.gz

FROM scratch AS binary-releaser
COPY --from=planet-builder /planet /

FROM scratch AS releaser
COPY --from=tarball-builder /planet.tar.gz /

FROM releaser
