FROM planet/os

ARG SECCOMP_VER
ARG DOCKER_VER

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
        dnsmasq ; \
    apt-get -t testing install -y lvm2; \
    apt-get -y autoclean; apt-get -y clean

# do not install docker from Debian repositories but rather download static binaries for seccomp support
RUN curl https://get.docker.com/builds/Linux/x86_64/docker-$DOCKER_VER.tgz -o /tmp/docker-$DOCKER_VER.tgz && \
    tar -xvzf /tmp/docker-$DOCKER_VER.tgz -C /tmp && \
    cp /tmp/docker/* /usr/bin && \
    rm -rf /tmp/docker*

RUN groupadd --system --non-unique --gid 1000 planet ;\
    useradd --system --non-unique --no-create-home -g 1000 -u 1000 planet
