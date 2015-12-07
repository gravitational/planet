#!/bin/bash

# Exit at first encountered error
trap 'exit' ERR

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

function build-success-status {
  local status=()
  kubectl_num_status_nodes=$(kubectl get cs -o go-template --template="{{len .items}}")

  for (( i=$kubectl_num_status_nodes; i>0; i-- )) do
    status+="True"
  done
  echo ${status[@]}
}

function cluster-status {
  kubectl get cs -o go-template --template="{{range .items}}{{range .conditions}}{{.status}}{{end}}{{end}}"
}

function wait-for-apiserver {
  echo "waiting for api-server"

  local n=60
  while [ -n "$(kubectl version >/dev/null 2>&1)" ] && [ "$n" -gt 0 ]; do
    echo "failed to query api-server version"
    n=$(($n-1))
    sleep 5
  done

  if [ "$n" -eq 0 ]; then
    exit 1
  fi
}

function wait-for-cluster {
  echo "waiting for cluster to become healthy"

  local success=$(build-success-status)
  local n=60
  while [ "$(cluster-status)" != "$success" ] && [ "$n" -gt 0 ]; do
    echo "cluster status is unhealthy"
    n=$(($n-1))
    sleep 5
  done

  if [ "$n" -eq 0 ]; then
    exit 2
  fi
}

function create-kube-namespace {
  kubectl get namespace kube-system >/dev/null 2>&1
  if [ "$?" != "0" ]; then
    kubectl create -f <(echo "$KUBE_NS")
  fi
}

function create-etcd-service {
  kubectl get svc etcd --namespace=kube-system >/dev/null 2>&1
  if [ "$?" = "0" ]; then
    kubectl delete -f <(echo "$ETCD_SVC") >/dev/null 2>&1
  fi
  kubectl create -f <(echo "$ETCD_SVC")
}

function create-kube-dns-service {
  kubectl get svc kube-dns --namespace=kube-system >/dev/null 2>&1
  if [ "$?" = "0" ]; then
    kubectl delete -f <(echo "$KUBE_DNS_SVC") >/dev/null 2>&1
  fi
  kubectl create -f <(echo "$KUBE_DNS_SVC")
}

wait-for-apiserver
wait-for-cluster

trap - ERR

create-kube-namespace
create-etcd-service
create-kube-dns-service
