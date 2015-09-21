#!/bin/bash
#
# this script runs inside of 'buildbox' docker conatiner which has
# /build volume mounted to 'build' directory here
#
# it builds shared modules used by all planet builds, with the output 
# going to $OUT (/build/out)

if [ -z "$SHAREDK8" ]; then 

echo -e "\n**** building flannel ****\n"
make -C $BUILDDIR/makefiles/base/network -f network.mk

echo -e "\n**** getting docker ****\n"
make -C $BUILDDIR/makefiles/base/docker -f docker.mk 

echo -e "\n**** getting registry ****\n"
make -C $BUILDDIR/makefiles/registry -f registry.mk

echo -e "\n**** building kubernetes ****\n"
make -C $BUILDDIR/makefiles/kubernetes -f kubernetes.mk

echo -e "\n**** removing extra crap from rootfs ****\n"
rm -rfv $ROOTFS/usr/share/doc
rm -rfv $ROOTFS/usr/share/man
rm -rfv $ROOTFS/var/cache/man
rm -rfv $ROOTFS/var/cache/debconf/*
rm -rfv $ROOTFS/var/cache/apt/archives/*
for lang in "fr es de da cs ca bg hu ja it id lg lt nb nl pl pt ru ro sksl sr sv th tl tr uk vi zh_CN zh_TW" ; do
    rm -rfv $ROOTFS/usr/share/locale/$lang
done

SHAREDK8="yes"
else
    echo -e "\n\n**** base is already done! ***\n"
fi
