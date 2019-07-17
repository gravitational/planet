FROM planet/os

ARG DOCKER_VER
ARG HELM_VER

RUN apt-get update && apt-get install -q -y bridge-utils \
    seccomp \
    bash-completion \
    kmod \
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
    lvm2 \
    dnsmasq && \
    apt-get -y autoclean && apt-get -y clean

# do not install docker from Debian repositories but rather download static binaries for seccomp support
RUN curl https://download.docker.com/linux/static/stable/x86_64/docker-$DOCKER_VER-ce.tgz -o /tmp/docker-$DOCKER_VER.tgz && \
    tar -xvzf /tmp/docker-$DOCKER_VER.tgz -C /tmp && \
    cp /tmp/docker/* /usr/bin && \
    rm -rf /tmp/docker*

# Replace docker-runc (CVE-2019-5736)
RUN curl -L https://github.com/rancher/runc-cve/releases/download/CVE-2019-5736-build2/runc-v$DOCKER_VER-amd64 -o /usr/bin/docker-runc


RUN curl https://storage.googleapis.com/kubernetes-helm/helm-$HELM_VER-linux-amd64.tar.gz -o /tmp/helm-$HELM_VER.tar.gz && \
    mkdir -p /tmp/helm && tar -xvzf /tmp/helm-$HELM_VER.tar.gz -C /tmp/helm && \
    cp /tmp/helm/linux-amd64/helm /usr/bin && \
    rm -rf /tmp/helm*

RUN groupadd --system --non-unique --gid 1000 planet ;\
    useradd --system --non-unique --no-create-home -g 1000 -u 1000 planet
