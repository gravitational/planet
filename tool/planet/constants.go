package main

import (
	"time"
)

const (
	EnvMasterIP                = "KUBE_MASTER_IP"
	EnvCloudProvider           = "KUBE_CLOUD_PROVIDER"
	EnvServiceSubnet           = "KUBE_SERVICE_SUBNET"
	EnvPODSubnet               = "KUBE_POD_SUBNET"
	EnvPublicIP                = "PLANET_PUBLIC_IP"
	EnvClusterDNSIP            = "KUBE_CLUSTER_DNS_IP"
	EnvAPIServerName           = "KUBE_APISERVER"
	EnvEtcdMemberName          = "ETCD_MEMBER_NAME"
	EnvEtcdInitialCluster      = "ETCD_INITIAL_CLUSTER"
	EnvEtcdInitialClusterState = "ETCD_INITIAL_CLUSTER_STATE"
	EnvLeaderKey               = "KUBE_LEADER_KEY"
	EnvRole                    = "PLANET_ROLE"
	EnvClusterID               = "KUBE_CLUSTER_ID"
	DefaultLeaderTerm          = 10 * time.Second
	DefaultEtcdEndpoints       = "http://127.0.0.1:2379"
)
