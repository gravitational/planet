# This dockerfile bakes a base image Planet will use.
# It is basically Debian with latest packages and properly configured locales
#
FROM debian:jessie-backports

ENV DEBIAN_FRONTEND noninteractive

# Locales
ADD locale.gen /etc/locale.gen
ADD profile /etc/profile
RUN echo 'deb http://ftp.debian.org/debian jessie-backports main' >> /etc/apt/sources.list && \
	echo 'deb http://httpredir.debian.org/debian/ jessie main contrib non-free' >> /etc/apt/sources.list && \
	echo 'deb http://httpredir.debian.org/debian/ jessie-updates main contrib non-free' >> /etc/apt/sources.list
RUN (apt-get clean \
    && apt-key update \
	&& apt-get -q -y update --fix-missing \
    && apt-get -q -y update \
	&& apt-get install -q -y apt-utils \
	&& apt-get install -q -y less \
	&& apt-get install -q -y locales \
	&& apt-get -t jessie-backports install -q -y systemd)

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
