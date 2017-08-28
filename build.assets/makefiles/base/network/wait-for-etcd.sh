#!/bin/bash

n=0
until [ $n -ge 10 ]
do
    /usr/bin/etcdctl \
      --cert-file=/var/state/etcd.cert \
      --key-file=/var/state/etcd.key \
      --ca-file=/var/state/root.cert \
      --timeout="5s" \
      --total-timeout="30s" \
      --peers https://127.0.0.1:4001 cluster-health && exit 0
    n=$[$n+1]
    sleep 3
done
