#!/bin/bash

# this script runs inside of 'buildbox' docker conatiner which has
# /build volume mounted to 'build' directory here
#
# it builds multiple projects all of them are going to $OUT which
# is set to /build/out
mkdir -p $OUT

echo -e "\n**** building flannel ****\n"
make -C $BUILDDIR/makefiles/base/network -f network.mk

echo -e "\n**** getting docker ****\n"
make -C $BUILDDIR/makefiles/base/docker -f docker.mk 

echo -e "\n**** getting registry ****\n"
make -C $BUILDDIR/makefiles/registry -f registry.mk

echo -e "\n**** building kubernetes ****\n"
make -C $BUILDDIR/makefiles/kubernetes -f kubernetes.mk

echo -e "\n**** building etcd ****\n"
make -C $BUILDDIR/makefiles/master/etcd -f etcd.mk

echo -e "\n**** building k8s-master ****\n"
make -C $BUILDDIR/makefiles/master/k8s-master -f k8s-master.mk

echo -e "\n**** building k8s-node ****\n"
make -C $BUILDDIR/makefiles/node/k8s-node -f k8s-node.mk
