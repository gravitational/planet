FROM planet/os

ARG SECCOMP_VER
ARG DOCKER_VER
ARG HELM_VER
ARG COREDNS_VER

# FIXME: allowing downgrades and pinning the version of libip4tc0 for iptables
# as the package has a dependency on the older version as the one available.
RUN apt-get update && apt-get install -q -y --allow-downgrades bridge-utils \
        seccomp=$SECCOMP_VER \
        bash-completion \
        kmod \
        libip4tc0=1.6.0+snapshot20161117-6 \
        iptables \
        ebtables \
        libdevmapper1.02.1 \
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
        coreutils \
        lsof \
        socat \
        nmap \
        netcat \
        nfs-common \
        jq \
        conntrack \
        strace \
        procps \
        lvm2 && \
    apt-get -y autoclean && apt-get -y clean

# do not install docker from Debian repositories but rather download static binaries for seccomp support
RUN curl https://download.docker.com/linux/static/stable/x86_64/docker-$DOCKER_VER-ce.tgz -o /tmp/docker-$DOCKER_VER.tgz && \
    tar -xvzf /tmp/docker-$DOCKER_VER.tgz -C /tmp && \
    cp /tmp/docker/* /usr/bin && \
    rm -rf /tmp/docker*

RUN curl https://storage.googleapis.com/kubernetes-helm/helm-$HELM_VER-linux-amd64.tar.gz -o /tmp/helm-$HELM_VER.tar.gz && \
    mkdir -p /tmp/helm && tar -xvzf /tmp/helm-$HELM_VER.tar.gz -C /tmp/helm && \
    cp /tmp/helm/linux-amd64/helm /usr/bin && \
    rm -rf /tmp/helm*

RUN curl -L https://github.com/coredns/coredns/releases/download/v${COREDNS_VER}/coredns_${COREDNS_VER}_linux_amd64.tgz -o /tmp/coredns-${COREDNS_VER}.tar.gz && \
    mkdir -p /tmp/coredns && tar -xvzf /tmp/coredns-${COREDNS_VER}.tar.gz -C /tmp/coredns && \
    cp /tmp/coredns/coredns /usr/bin && \
    rm -rf /tmp/coredns*

RUN groupadd --system --non-unique --gid 1000 planet ;\
    useradd --system --non-unique --no-create-home -g 1000 -u 1000 planet
