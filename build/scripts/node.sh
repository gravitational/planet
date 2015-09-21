#!/bin/bash
#
# this script runs inside of 'buildbox' docker conatiner which has
# /build volume mounted to 'build' directory here
#
# it builds 'node' pieces of k8s, with the output going to $OUT (/build/out)
bash $BUILDDIR/scripts/base.sh

echo -e "\n**** building etcdctl  ****\n"
make -C $BUILDDIR/makefiles/node/etcdctl -f etcdctl.mk

echo -e "\n**** building k8s-node ****\n"
make -C $BUILDDIR/makefiles/node/k8s-node -f k8s-node.mk
