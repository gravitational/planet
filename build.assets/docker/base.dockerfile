FROM planet/os

RUN apt-get install -q -y bridge-utils \
        kmod \
        iptables \
        libdevmapper1.02.1 \
        libsqlite3-0 \
        e2fsprogs \
        libncurses5 \
        net-tools \
        iproute2 \
        lsb-base \
        dash \
        openssl

RUN useradd -MUr -u 1000 planet
