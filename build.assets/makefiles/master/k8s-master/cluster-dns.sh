#!/bin/bash

# Prior to upgrade to Kubernetes 1.19 the list of cluster DNS servers are provided
# through the kubelet flag `--cluster-dns`. This flag is now deprecated and the cluster
# DNS servers must be specified via the kubelet config file under `clusterDNS`.
#
# This script writes the cluster DNS servers into the kubelet config file.
#
# More info on `--cluster-dns` can be found at: https://v1-19.docs.kubernetes.io/docs/reference/command-line-tools-reference/kubelet/

# Environment file that contains the cluster DNS_ADDRESSES.
DNS_ENV=/run/dns.env
# Path to kubelet config file.
KUBELET_CONFIG=/etc/kubernetes/kubelet.yaml

# If /run/dns.env file does not exist exit without any changes.
if [ ! -f $DNS_ENV ]
then
    echo "$DNS_ENV does not exist"
    exit 1
fi

# Import DNS_ADDRESSES from env file.
export $(cat $DNS_ENV | xargs)

# If DNS_ADDRESSES is empty exit without any changes.
if [ -z "$DNS_ADDRESSES" ]
then
    echo "DNS_ADDRESSES is not set"
    exit 1
fi

# Remove existing clusterDNS values.
# clusterDNS contains a list of addresses. Example:
#
# clusterDNS:
#   - 100.100.100.101
#   - 100.100.100.102
#   - 100.100.100.103
sed -n -i '/clusterDNS:/{g; :a; n; /^[^[:space:]]/!ba};p' $KUBELET_CONFIG

# Write new clusterDNS values to kubelet config.
CLUSTER_DNS="clusterDNS:"
while IFS=',' read -ra ADDR; do
    for i in "${ADDR[@]}"; do
        CLUSTER_DNS+="\n  - $i"
    done
done <<< "$DNS_ADDRESSES"
echo -e "$CLUSTER_DNS" | tee -a $KUBELET_CONFIG