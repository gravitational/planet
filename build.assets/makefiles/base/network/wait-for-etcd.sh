#!/bin/bash

# Prior to upgrade to etcd3 the cluster-health command would return "healthy"
# result as long as the cluster could reach quorum. For example, if 1 out of
# 3 nodes was down, the cluster would be considered "healthy".
#
# etcd3 changed this behavior by returning "cluster is degraded" if any of
# the nodes are unavailable, and "cluster is unavailable" if quorum can't
# be reached.
#
# Hence this script treats both "healthy" and "degraded" states as "healthy"
# to keep old etcd2 behavior that some services rely on.
#
# Here's the relevant etcd commit: https://github.com/etcd-io/etcd/commit/ad0b3cfdab859c2d29c2605443c9820f44d34ae5.

PEERS=${1:-https://127.0.0.1:2379}

n=0
until [ $n -ge 10 ]
do
    if /usr/bin/etcdctl \
      --cert-file=/var/state/etcd.cert \
      --key-file=/var/state/etcd.key \
      --ca-file=/var/state/root.cert \
      --timeout="5s" \
      --total-timeout="30s" \
      --peers ${PEERS} cluster-health | grep -E 'cluster is healthy|cluster is degraded'; then
        exit 0
    fi
    n=$[$n+1]
    sleep 3
done
