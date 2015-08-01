#!/bin/bash

mkdir -p /tmp/external
cat << EOF > /tmp/external/container-environment
KUBE_MASTER_IP=127.0.0.1
KUBE_CLOUD_PROVIDER=aws
EOF


sudo mkdir -p /tmp/etcd
sudo chown alex:alex /tmp/etcd/
sudo docker run -i -t --privileged --net=host --volume=/tmp/etcd:/var/etcd --volume=/tmp/external:/etc/external --volume=/lib/modules:/lib/modules grv.io/cube:latest 
