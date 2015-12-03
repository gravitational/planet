#!/bin/bash

MASTER_IP=$1
CLUSTER_DNS_IP=$2

ETCD_SVC=`cat <<-EOM
kind: Service
apiVersion: v1
metadata:
  name: etcd
  namespace: kube-system
spec:
  ports:
  - protocol: TCP
    name: client
    port: 4001
    targetPort: 4001
---
kind: Endpoints
apiVersion: v1
metadata:
  name: etcd
  namespace: kube-system
subsets:
  - addresses:
    - ip: $MASTER_IP
    ports:
    - port: 4001
      name: client
EOM
`

KUBE_NS=`cat <<-EOM
apiVersion: v1
kind: Namespace
metadata:
  name: kube-system
EOM
`

KUBE_DNS_SVC=`cat <<-EOM
apiVersion: v1
kind: Service
metadata:
  name: kube-dns
  namespace: kube-system
  labels:
    k8s-app: kube-dns
    kubernetes.io/cluster-service: "true"
    kubernetes.io/name: "KubeDNS"
spec:
  selector:
    k8s-app: kube-dns
  clusterIP: $CLUSTER_DNS_IP
  ports:
  - name: dns
    port: 53
    protocol: UDP
  - name: dns-tcp
    port: 53
    protocol: TCP
EOM
`

# create kube-system namespace
kubectl get namespace kube-system
if [ "$?" != "0" ]; then
  kubectl create -f  <(echo "$KUBE_NS")
fi

# create etcd service/endpoint
kubectl get svc etcd --namespace=kube-system
if [ "$?" != "0" ]; then
  kubectl create -f <(echo "$ETCD_SVC")
fi

# create kube-dns service
kubectl get svc kube-dns --namespace=kube-system
if [ "$?" != "0" ]; then
  kubectl create -f <(echo "$KUBE_DNS_SVC")
fi
