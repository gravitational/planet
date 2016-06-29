#!/bin/bash

# set real k8s service IP
sed -i "s/KUBE_CLUSTER_DNS_IP/${KUBE_CLUSTER_DNS_IP}/g" /etc/dnsmasq.d/k8s.conf
