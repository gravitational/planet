# This dockerfile bakes a base image Planet will use.
# It is basically Debian with latest packages and properly configured locales
#
FROM debian:jessie

ENV DEBIAN_FRONTEND noninteractive

# Locales
ADD locale.gen /etc/locale.gen
ADD profile /etc/profile
RUN (apt-get clean \
    && apt-key update \
	&& apt-get -q -y update --fix-missing \
    && apt-get -q -y update \
	&& apt-get install -q -y apt-utils \
	&& apt-get install -q -y less \
	&& apt-get install -q -y locales)

# Set locale to en_US.UTF-8
RUN (locale-gen \
	&& locale-gen en_US.UTF-8 \
	&& dpkg-reconfigure locales)

RUN systemctl set-default multi-user.target

ENV LANGUAGE en_US.UTF-8
ENV LANG en_US.UTF-8
ENV LC_ALL en_US.UTF-8
ENV LC_CTYPE en_US.UTF-8

ENV PAGER /usr/bin/less
ENV LESS -isM
