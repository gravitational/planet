#!/bin/bash
source /etc/container-environment

until [ -e /run/flannel/subnet.env ]; 
  do echo \"waiting for flannel to start.\"; 
  sleep 3; 
done; 

# https://github.com/gravitational/gravity.e/issues/3898
# The node may not have a default gateway or routes that cover the service subnet
# This prevents "no route to host" errors when trying to reach the service subnet
# by creating a dummy interface, and routing the service subnet to this interface
# we can guarentee that the route exists, and can be NAT to the correct destination
ip link add flannel.null type dummy
ip link set flannel.null up
ip route add ${KUBE_SERVICE_SUBNET} dev flannel.null
exit 0