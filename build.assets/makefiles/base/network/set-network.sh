#!/bin/bash

n=0
until [ $n -ge 10 ]
do
    /usr/bin/etcdctl set /coreos.com/network/config '{"Network":"10.244.0.0/16", "Backend": {"Type": "vxlan"}}' && exit 0
    n=$[$n+1]
    echo "Failed to set variable reconnecting to the cluster"
    sleep 1
done


