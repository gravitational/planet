#!/bin/bash

# this script runs inside of 'buildbox' docker conatiner which has
# /build volume mounted to 'build' directory here
#
# it builds multiple projects all of them are going to $OUT which
# is set to /build/out
mkdir -p $OUT

bash $BUILDDIR/scripts/base.sh
bash $BUILDDIR/scripts/master.sh
bash $BUILDDIR/scripts/node.sh
