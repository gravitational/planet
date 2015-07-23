#!/bin/bash

n=0
until [ $n -ge 10 ]
do
    /usr/bin/etcdctl cluster-health  && exit 0
    n=$[$n+1]
    echo "Failed to set variable reconnecting to the cluster"
    sleep 1
done
