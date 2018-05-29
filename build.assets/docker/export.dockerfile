FROM scratch
LABEL maintainer="Gravitational, Inc"

ADD /rootfs /
CMD ["/bin/bash"]
