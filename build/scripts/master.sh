#!/bin/bash
#
# this script runs inside of 'buildbox' docker conatiner which has
# /build volume mounted to 'build' directory here
#
# it builds 'master' pieces of k8s, with the output going to $OUT (/build/out)
bash $BUILDDIR/scripts/base.sh

echo -e "\n**** building etcd ****\n"
make -C $BUILDDIR/makefiles/master/etcd -f etcd.mk

echo -e "\n**** building k8s-master ****\n"
make -C $BUILDDIR/makefiles/master/k8s-master -f k8s-master.mk
