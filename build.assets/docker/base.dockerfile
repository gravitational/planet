FROM planet/os

RUN apt-get update && apt-get install -q -y bridge-utils \
        bash-completion \
        kmod \
        iptables \
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
        dnsmasq ; \
    apt-get -y autoclean; apt-get -y clean

RUN groupadd --system --non-unique --gid 1000 planet ;\
    useradd --system --non-unique --no-create-home -g 1000 -u 1000 planet
