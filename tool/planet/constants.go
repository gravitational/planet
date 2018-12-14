/*
Copyright 2018 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
	EnvEtcdProxy               = "ETCD_PROXY"
	EnvEtcdMemberName          = "ETCD_MEMBER_NAME"
	EnvEtcdInitialCluster      = "ETCD_INITIAL_CLUSTER"
	EnvEtcdInitialClusterState = "ETCD_INITIAL_CLUSTER_STATE"
	EnvEtcdctlCertFile         = "ETCDCTL_CERT_FILE"
	EnvEtcdctlKeyFile          = "ETCDCTL_KEY_FILE"
	EnvEtcdctlCAFile           = "ETCDCTL_CA_FILE"
	EnvEtcdctlPeers            = "ETCDCTL_PEERS"
	EnvLeaderKey               = "KUBE_LEADER_KEY"
	EnvRole                    = "PLANET_ROLE"
	EnvElectionEnabled         = "PLANET_ELECTION_ENABLED"
	EnvClusterID               = "KUBE_CLUSTER_ID"
	EnvNodeName                = "KUBE_NODE_NAME"
	EnvAgentName               = "PLANET_AGENT_NAME"
	EnvInitialCluster          = "PLANET_INITIAL_CLUSTER"
	EnvStateDir                = "PLANET_STATE_DIR"
	EnvAWSAccessKey            = "AWS_ACCESS_KEY_ID"
	EnvAWSSecretKey            = "AWS_SECRET_ACCESS_KEY"
	EnvKubeConfig              = "KUBECONFIG"

	PlanetRoleMaster = "master"

	EtcdProxyOn  = "on"
	EtcdProxyOff = "off"

	DefaultLeaderTerm    = 10 * time.Second
	DefaultEtcdEndpoints = "https://127.0.0.1:2379"

	DefaultSecretsMountDir = "/var/state"
	DefaultEtcdctlCertFile = DefaultSecretsMountDir + "/etcd.cert"
	DefaultEtcdctlKeyFile  = DefaultSecretsMountDir + "/etcd.key"
	DefaultEtcdctlCAFile   = DefaultSecretsMountDir + "/root.cert"

	// APIServerDNSName defines the DNS entry name of the master node
	APIServerDNSName = "apiserver"

	// CloudProviderAWS defines the name of the AWS cloud provider used to
	// setup AWS integration in kubernetes
	CloudProviderAWS = "aws"

	// DNSNdots is the amount of NDOTS we set before doing initial global query
	DNSNdots = 2
	// DNSTimeout is the amount of seconds to wait
	DNSTimeout = 1

	// LocalDNSIP is the IP of the local DNS server
	LocalDNSIP = "127.0.0.1"

	// ETCDServiceName names the service unit for etcd
	ETCDServiceName = "etcd.service"
	// APIServerServiceName names the service unit for k8s apiserver
	APIServerServiceName = "kube-apiserver.service"

	// PlanetResolv is planet local resolver
	PlanetResolv = "resolv.gravity.conf"
	// KubeletResolv is kubelet local resolver
	KubeletResolv = "resolv.kubelet.conf"
	// SharedFileMask is file mask for shared file
	SharedFileMask = 0644

	// DNSMasqK8sConf is DNSMasq DNS server K8s config
	DNSMasqK8sConf = "/etc/dnsmasq.d/k8s.conf"

	// DNSMasqAPIServerConf is the dnsmasq configuration file for apiserver
	DNSMasqAPIServerConf = "/etc/dnsmasq.d/apiserver.conf"

	// KubeConfigPath defines a path to a privileged kube config file
	KubeConfigPath = "/etc/kubernetes/scheduler.kubeconfig"

	// UnprivilegedKubeConfigPath defines a path to an unprivileged kube config file
	UnprivilegedKubeConfigPath = "/etc/kubernetes/kubelet.kubeconfig"
)

// K8sSearchDomains are default k8s search domain settings
var K8sSearchDomains = []string{
	"svc.cluster.local",
	"default.svc.cluster.local",
	"kube-system.svc.cluster.local",
}
