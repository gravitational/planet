# This dockerfile bakes a base image Planet will use.
# It is basically Debian with latest packages and properly configured locales
#
FROM debian:stretch-backports

ENV DEBIAN_FRONTEND noninteractive

ADD os-rootfs/ /

RUN set -ex; \
	if ! command -v gpg > /dev/null; then \
	apt-get update; \
	apt-get install -y --no-install-recommends \
	gnupg2 \
	dirmngr \
	; \
	rm -rf /var/lib/apt/lists/*; \
	fi

RUN sed -i 's/main/main contrib non-free/g' /etc/apt/sources.list && \
	apt-get update && apt-get -q -y install apt-transport-https \
	&& apt-get install -q -y apt-utils less locales \
	&& apt-get install -t stretch-backports -q -y systemd \
	&& rm -rf /var/lib/apt/lists/*

# Set locale to en_US.UTF-8
RUN (locale-gen \
	&& locale-gen en_US.UTF-8 \
	&& dpkg-reconfigure locales)

# https://github.com/systemd/systemd/blob/v230/src/shared/install.c#L413
# Exit code 1 is either created successfully or symlink already exists
# Exit codes < 0 are failures
RUN systemctl set-default multi-user.target; if [ "$?" -lt 0 ]; then exit $?; fi;

ENV LANGUAGE en_US.UTF-8
ENV LANG en_US.UTF-8
ENV LC_ALL en_US.UTF-8
ENV LC_CTYPE en_US.UTF-8

ENV PAGER /usr/bin/less
ENV LESS -isM
