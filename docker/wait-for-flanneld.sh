#!/bin/bash

n=0
until [ $n -ge 10 ]
do
    cat /run/flannel/subnet.env && exit 0
    n=$[$n+1]
    echo "Waiting for flanneld to start up"
    sleep 1
done
