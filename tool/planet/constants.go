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

	"github.com/gravitational/planet/lib/clusterconfig/component"
	"github.com/gravitational/planet/lib/utils"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// EnvMasterIP names the environment variable that specifies
	// the IP address of the leader node
	EnvMasterIP = "KUBE_MASTER_IP"
	// EnvCloudProvider names the environment variable that specifies
	// the name of the cloud provider
	EnvCloudProvider = "KUBE_CLOUD_PROVIDER"
	// EnvServiceSubnet names the environment variable that specifies
	// the subnet CIDR for k8s services
	EnvServiceSubnet = "KUBE_SERVICE_SUBNET"
	// EnvPodSubnet names the environment variable that specifies
	// the subnet CIDR for k8s pods
	EnvPodSubnet = "KUBE_POD_SUBNET"
	// EnvServiceNodePortRange defines the range of ports to reserve for services
	// with NodePort visibility. Inclusive at both ends of the range
	EnvServiceNodePortRange = "KUBE_SERVICE_NODE_PORT_RANGE"
	// EnvProxyPortRange specifies the range of host ports (beginPort-endPort, single port
	// or beginPort+offset, inclusive) that may be consumed in order to proxy service traffic.
	// If (unspecified, 0, or 0-0) then ports will be randomly chosen.
	EnvProxyPortRange = "KUBE_PROXY_PORT_RANGE"
	// EnvFeatureGates specifies the set of key=value pairs that describe feature gates for
	// alpha/experimental features inside the runtime container
	EnvFeatureGates = "KUBE_FEATURE_GATES"
	// EnvVxlanPort is the environment variable with overlay network port
	EnvVxlanPort = "PLANET_VXLAN_PORT"
	// EnvStorageBackend names the environment variable that specifies
	// which storage backend kubernetes should use (etcd2/etcd3)
	EnvStorageBackend = "KUBE_STORAGE_BACKEND"
	// EnvPublicIP names the environment variable that specifies
	// the public IP address of the node
	EnvPublicIP = "PLANET_PUBLIC_IP"
	// EnvClusterDNSIP names the environment variable that specifies
	// the IP address of the k8s DNS service
	EnvClusterDNSIP = "KUBE_CLUSTER_DNS_IP"
	// EnvAPIServerName names the environment variable that specifies
	// the address of the API server
	EnvAPIServerName = "KUBE_APISERVER"
	// See https://coreos.com/etcd/docs/latest/v2/configuration.html
	// EnvEtcdProxy names the environment variable that specifies
	// the value of the proxy mode setting
	EnvEtcdProxy = "PLANET_ETCD_PROXY"
	// EnvEtcdMemberName names the environment variable that specifies
	// the name of this node in the etcd cluster
	EnvEtcdMemberName = "PLANET_ETCD_MEMBER_NAME"
	// EnvEtcdInitialClusterState names the environment variable that specifies
	// the initial etcd cluster configuration for bootstrapping
	EnvEtcdInitialCluster = "ETCD_INITIAL_CLUSTER"
	// EnvEtcdGatewayEndpoints is a list of endpoints the etcd gateway can use
	// to reach the etcd cluster
	EnvEtcdGatewayEndpoints = "PLANET_ETCD_GW_ENDPOINTS"
	// EnvEtcdInitialClusterState names the environment variable that specifies
	// the initial etcd cluster state
	EnvEtcdInitialClusterState = "ETCD_INITIAL_CLUSTER_STATE"
	// EnvEtcdVersion names the environment variable that specifies
	// the version of etcd to use
	EnvEtcdVersion = "PLANET_ETCD_VERSION"
	// EnvEtcdPrevVersion points to the previously installed version used for rollback
	EnvEtcdPrevVersion = "PLANET_ETCD_PREV_VERSION"
	// EnvEtcdctlCertFile names the environment variable that specifies the location
	// of the certificate file
	EnvEtcdctlCertFile = "ETCDCTL_CERT_FILE"
	// EnvEtcdctlKeyFile names the environment variable that specifies the location
	// of the certificate key file
	EnvEtcdctlKeyFile = "ETCDCTL_KEY_FILE"
	// EnvEtcdctlCAFile names the environment variable that specifies the location
	// of the CA certificate file
	EnvEtcdctlCAFile = "ETCDCTL_CA_FILE"
	// EnvEtcdctlPeers names the environment variable that specifies the list of nodes
	// in the etcd cluster as a comma-separated list
	EnvEtcdctlPeers = "ETCDCTL_PEERS"

	// EnvLeaderKey names the environment variable that specifies the name
	// of the key with the active leader
	EnvLeaderKey = "KUBE_LEADER_KEY"
	// EnvRole names the environment variable that specifies the service role of this node
	// (master or not)
	EnvRole = "PLANET_ROLE"
	// EnvElectionEnabled names the environment variable that controls if this
	// node is participating in leader election when it starts
	EnvElectionEnabled = "PLANET_ELECTION_ENABLED"
	// EnvClusterID names the environment variable that is the name of the cluster
	EnvClusterID = "KUBE_CLUSTER_ID"
	// EnvNodeName names the environment variable that overrides the
	// hostname attributes for k8s kubelet/kube-proxy
	EnvNodeName = "KUBE_NODE_NAME"
	// EnvAgentName names the environment variable that specifies the name
	// of the planet agent as known within the serf cluster
	EnvAgentName = "PLANET_AGENT_NAME"
	// EnvInitialCluster names the environment variable that specifies the initial
	// agent cluster configuration as comma-separated list of addresses
	EnvInitialCluster = "PLANET_INITIAL_CLUSTER"
	// EnvAWSAccessKey names the environment variable that specifies the AWS
	// access key
	EnvAWSAccessKey = "AWS_ACCESS_KEY_ID"
	// EnvAWSSecretKey names the environment variable that specifies the AWS
	// secret access key
	EnvAWSSecretKey = "AWS_SECRET_ACCESS_KEY"
	// EnvKubeConfig names the environment variable that specifies location
	// of the kubernetes configuration file
	EnvKubeConfig = "KUBECONFIG"
	// EnvDNSHosts is the environment variable that specifies DNS hostname
	// overrides for the CoreDNS config
	EnvDNSHosts = "PLANET_DNS_HOSTS"
	// EnvDNSZones is the environment variable that specified DNS zone
	// overrides for the CoreDNS config
	EnvDNSZones = "PLANET_DNS_ZONES"
	// EnvHostname names the environment variable that specifies the new
	// hostname
	EnvHostname = "PLANET_HOSTNAME"
	// EnvDNSUpstreamNameservers names the environment variable that specifies
	// additional nameservers to add to the container's CoreDNS configuration
	EnvDNSUpstreamNameservers = "PLANET_DNS_UPSTREAM_NAMESERVERS"
	// EnvDockerOptions names the environment variable that specifies additional
	// command line options for docker
	EnvDockerOptions = "DOCKER_OPTS"
	// EnvEtcdOptions names the environment variable that specifies additional etcd
	// command line options
	EnvEtcdOptions = "ETCD_OPTS"

	// EnvKubeletOptions names the environment variable that specifies additional
	// kubelet command line options
	EnvKubeletOptions = "KUBE_KUBELET_FLAGS"

	// EnvAPIServerOptions names the environment variable that specifies additional
	// command line options for the API server
	EnvAPIServerOptions = "KUBE_APISERVER_FLAGS"

	// EnvKubeProxyOptions specifies additional command line options for kube-proxy
	EnvKubeProxyOptions = "KUBE_PROXY_FLAGS"

	// EnvControllerManagerOptions specifies additional command line options for controller manager
	EnvControllerManagerOptions = "KUBE_CONTROLLER_MANAGER_FLAGS"

	// EnvKubeCloudFlags specifies cloud configuration command line options
	EnvKubeCloudFlags = "KUBE_CLOUD_FLAGS"

	// EnvKubeComponentFlags specifies command line options common to all components
	EnvKubeComponentFlags = "KUBE_COMPONENT_FLAGS"

	// EnvOverlayAddresses is an environment variable with a comma separated list of
	// IPv4 addresses assigned to the overlay network interface of the host
	EnvOverlayAddresses = "OVERLAY_ADDRESSES"

	// EnvPath is the PATH environment variable
	EnvPath = "PATH"

	// EnvPlanetAgentCAFile names the environment variable that specifies the location
	// of the agent ca certificate file
	EnvPlanetAgentCAFile = "PLANET_AGENT_CAFILE"

	// EnvPlanetAgentClientCertFile names the environment variable that specifies the location
	// of the agent certificate file
	EnvPlanetAgentClientCertFile = "PLANET_AGENT_CLIENT_CERTFILE"

	// EnvPlanetAgentClientKeyFile names the environment variable that specifies the location
	// of the agent key file
	EnvPlanetAgentClientKeyFile = "PLANET_AGENT_CLIENT_KEYFILE"

	// EnvDockerPromiscuousMode names the environment variable that specifies the
	// promiscuous mode for docker
	EnvDockerPromiscuousMode = "PLANET_DOCKER_PROMISCUOUS_MODE"
	// EnvServiceGID names the environment variable that specifies the service user ID
	EnvServiceUID = "PLANET_SERVICE_UID"
	// EnvServiceGID names the environment variable that specifies the service group ID
	EnvServiceGID = "PLANET_SERVICE_GID"

	// EnvGCENodeTags names the environment variable that defines network node tags on GCE
	EnvGCENodeTags = "PLANET_GCE_NODE_TAGS"

	// EnvPlanetKubeletOptions is the environment variable with additional options for kubelet.
	// This is the external configuration for the runtime container
	EnvPlanetKubeletOptions = "PLANET_KUBELET_OPTIONS"

	// EnvPlanetAPIServerOptions is the environment variable with additional options for API server.
	// This is the external configuration for the runtime container
	EnvPlanetAPIServerOptions = "PLANET_APISERVER_OPTIONS"

	// EnvPlanetFeatureGates defines the set of key=value pairs that describe feature gates for
	// alpha/experimental features.
	// This is the external configuration for the runtime container
	EnvPlanetFeatureGates = "PLANET_FEATURE_GATES"

	// EnvPlanetProxyPortRange specifies the range of host ports (beginPort-endPort, single port
	// or beginPort+offset, inclusive) that may be consumed in order to proxy service traffic.
	// If (unspecified, 0, or 0-0) then ports will be randomly chosen.
	// This is the external configuration for the runtime container
	EnvPlanetProxyPortRange = "PLANET_PROXY_PORT_RANGE"

	// EnvPlanetServiceNodePortRange defines the range of ports to reserve for services
	// with NodePort visibility. Inclusive at both ends of the range.
	// This is the external configuration for the runtime container
	EnvPlanetServiceNodePortRange = "KUBE_SERVICE_NODE_PORT_RANGE"

	// EnvPlanetDNSListenAddr is the environment variable with the list of listen addresses for CoreDNS
	EnvPlanetDNSListenAddr = "PLANET_DNS_LISTEN_ADDR"

	// EnvPlanetDNSPort is the environment variable with the DNS port
	EnvPlanetDNSPort = "PLANET_DNS_PORT"

	// EnvPlanetTaints is an environment variable for Kubernetes taints to apply to the node during creation
	EnvPlanetTaints = "PLANET_NODE_TAINTS"

	// EnvPlanetNodeLabels is an environment variable for Kubernetes node labels
	EnvPlanetNodeLabels = "PLANET_NODE_LABELS"

	// EnvPlanetKubeletConfig is an environment variable for Kubernetes kubelet configuration file.
	// This is external configuration for the runtime container and is assumed to be base64-encoded
	EnvPlanetKubeletConfig = "PLANET_KUBELET_CONFIG"

	// EnvPlanetCloudConfig is an environment variable for Kubernetes cloud configuration file
	EnvPlanetCloudConfig = "PLANET_CLOUD_CONFIG"

	// DefaultDNSListenAddr is the default IP address CoreDNS will listen on
	DefaultDNSListenAddr = "127.0.0.2"

	// DNSPort is the default DNS port
	DNSPort = 53

	// DefaultEnvPath defines the default value for PATH environment variable
	// when executing commands inside the container
	DefaultEnvPath = "/usr/local/bin:/usr/local/sbin:/usr/bin:/usr/sbin:/bin:/sbin"

	// PlanetRoleMaster specifies the value of the node role to be master.
	// A master node runs additional runtime tests as well as additional
	// set of services
	PlanetRoleMaster = "master"

	// EtcdProxyOn specifies the value of the proxy mode that enables it
	// See https://coreos.com/etcd/docs/latest/v2/configuration.html
	EtcdProxyOn = "on"
	// EtcdProxyOff specifies the value of the proxy mode that disables it
	EtcdProxyOff = "off"

	// DefaultLeaderTerm specifies the time-to-live value for the key used in leader election.
	// It defines the maximum time the leader is maintained before the election is re-attempted.
	DefaultLeaderTerm = 10 * time.Second
	// DefaultEtcdEndpoints specifies the default etcd endpoint
	DefaultEtcdEndpoints = "https://127.0.0.1:2379"
	// DefaultEtcdUpgradeEndpoints specified the endpoint for the temporary etcd used during upgrades
	DefaultEtcdUpgradeEndpoints = "https://127.0.0.2:2379"

	// DefaultSecretsMountDir specifies the default location for certificates
	// as mapped inside the container
	DefaultSecretsMountDir = "/var/state"
	// DefaultEtcdctlCertFile is the path to the etcd certificate file
	DefaultEtcdctlCertFile = DefaultSecretsMountDir + "/etcd.cert"
	// DefaultEtcdctlKeyFile is the path to the etcd certificate key file
	DefaultEtcdctlKeyFile = DefaultSecretsMountDir + "/etcd.key"
	// DefaultEtcdctlCAFile is the path to the etcd CA certificate file
	DefaultEtcdctlCAFile = DefaultSecretsMountDir + "/root.cert"

	// DefaultEtcdStoreBase is the base path to etcd data on disk
	DefaultEtcdStoreBase = "/ext/etcd"
	// DefaultEtcdCurrentVersionFile is the file location that contains version information about the etcd datastore
	DefaultEtcdCurrentVersionFile = "/ext/etcd/etcd-version.txt"
	// DefaultPlanetReleaseFile is the planet file that indicates the latest available etcd version
	DefaultPlanetReleaseFile = "/etc/planet-release"

	// AssumeEtcdVersion is the etcd version we assume we're using if we're unable to locate the running version
	AssumeEtcdVersion = "v2.3.8"

	// LegacyAPIServerDNSName is the domain name of a current leader server
	// as used to be in previous versions.
	// This is kept for backwards-compatibility
	LegacyAPIServerDNSName = "apiserver"

	// See resolv.conf(5) on a Linux machine
	//
	// DNSNdots defines the threshold for amount of dots that must appear in a name
	// before an initial absolute query will be made
	DNSNdots = 2
	// DNSTimeout is the amount time resolver will wait for response before retrying
	// the query with a different name server. Measured in seconds
	DNSTimeout = 1

	// ETCDServiceName names the service unit for etcd
	ETCDServiceName = "etcd.service"
	// ETCDUpgradeServiceName is a temporary etcd service used only during upgrades
	ETCDUpgradeServiceName = "etcd-upgrade.service"
	// APIServerServiceName names the service unit for k8s apiserver
	APIServerServiceName = "kube-apiserver.service"
	// PlanetAgentServiceName is the name of the planet agent
	PlanetAgentServiceName = "planet-agent.service"

	// ETCDDropinPath is the location of the systemd dropin when etcd is in gateway mode
	ETCDGatewayDropinPath = "/etc/systemd/system/etcd.service.d/10-gateway.conf"

	// PlanetResolv is planet local resolver
	PlanetResolv = "resolv.gravity.conf"

	// SharedFileMask is file mask for shared file
	SharedFileMask = 0644

	// SharedDirMask is a permissions mask for a shared directory
	SharedDirMask = 0755

	// SharedReadWriteMask is a mask for a shared file with read/write access for everyone
	SharedReadWriteMask = 0666

	// CoreDNSConf is the location of the coredns configuration file within planet
	CoreDNSConf = "/etc/coredns/coredns.conf"

	// CoreDNSClusterConf is the location of the coredns configuration file for the overlay network
	// and updated via k8s configmap
	CoreDNSClusterConf = "/etc/coredns/configmaps/overlay.conf"

	// CoreDNSHosts is the location of a hosts file to be served by CoreDNS
	CoreDNSHosts = "/etc/coredns/coredns.hosts"

	// HostsFile specifies the location of the hosts configuration file
	HostsFile = "/etc/hosts"

	// HostnameFile specifies the location of the hostname configuration file
	HostnameFile = "/etc/hostname"

	// RootUID is the ID of the root user
	RootUID = 0
	// RootGID is the ID of the root group
	RootGID = 0

	// UsersDatabase is a file where Linux accounts information is stored
	UsersDatabase = "/etc/passwd"
	// GroupsDatabase is a file where Linux groups information is stored
	GroupsDatabase = "/etc/group"

	// AgentStatusTimeout specifies the default status query timeout
	AgentStatusTimeout = 5 * time.Second

	// ClientRPCCAPath specifies the path to the CA certificate for agent RPC
	ClientRPCCAPath = "/var/state/root.cert"

	// ClientRPCCertPath specifies the path to the CA certificate for agent RPC
	ClientRPCCertPath = "/var/state/planet-rpc-client.cert"

	// ClientRPCKeyPath specifies the path to the CA certificate for agent RPC
	ClientRPCKeyPath = "/var/state/planet-rpc-client.key"

	// DefaultDockerBridge specifies the default name of the docker bridge
	DefaultDockerBridge = "docker0"

	// DefaultDockerUnit specifies the name of the docker service unit file
	DefaultDockerUnit = "docker.service"

	// DockerPromiscuousModeDropIn names the drop-in file with promiscuous mode configuration
	// for docker bridge
	DockerPromiscuousModeDropIn = "99-docker-promisc.conf"

	// MinKernelVersion specifies the minimum kernel version on the host
	MinKernelVersion = 310

	// DefaultServiceSubnet specifies the subnet CIDR used for k8s Services by default
	DefaultServiceSubnet = "10.100.0.0/16"

	// DefaultPodSubnet specifies the subnet CIDR used for k8s Pods by default
	DefaultPodSubnet = "10.244.0.0/16"

	// DefaultVxlanPort is the default overlay network port
	DefaultVxlanPort = 8472

	// DefaultFeatureGates is the default set of component feature gates
	DefaultFeatureGates = "AllAlpha=true,APIResponseCompression=false,BoundServiceAccountTokenVolume=false"

	// DefaultServiceNodePortRange defines the default IP range for services with NodePort visibility
	DefaultServiceNodePortRange = "30000-32767"

	// OverlayEnvFile specifies the file location to write information about the overlay network
	// in use to be picked up by scripts
	OverlayEnvFile = "/run/overlay.env"

	// ServiceUser specifies the name of the service user as seen inside the container.
	// Service user inside the container will be mapped to an existing user (not necessarily
	// with the same name) on host
	ServiceUser string = "planet"
	// ServiceGroup specifies the name of the service group as seen inside the container.
	ServiceGroup string = "planet"

	// ETCDBackupTimeout specifies the timeout when attempting to backup/restore etcd
	ETCDBackupTimeout = 5 * time.Minute

	// ETCDRegistryPrefix is the etcd directory for the k8s api server data in etcd
	ETCDRegistryPrefix = "/registry"

	// WaitInterval is the amount of time to sleep between loops
	WaitInterval = 100 * time.Millisecond

	// ServiceTimeout is the amount of time when trying to start/stop a systemd service
	ServiceTimeout = 1 * time.Minute

	// EtcdUpgradeTimeout is the amount of time to wait for operations during the etcd upgrade
	EtcdUpgradeTimeout = 15 * time.Minute

	// HighWatermark is the disk usage percentage that is considered degrading
	HighWatermark = 80
)

// K8sSearchDomains are default k8s search domain settings
var K8sSearchDomains = []string{
	"svc.cluster.local",
	"default.svc.cluster.local",
	"kube-system.svc.cluster.local",
}

// KubeletConfig specifies the default kubelet configuration
var KubeletConfig = component.KubeletConfiguration{
	TypeMeta: metav1.TypeMeta{
		Kind:       "KubeletConfiguration",
		APIVersion: "kubelet.config.k8s.io/v1beta1",
	},
	Address:                "0.0.0.0",
	Port:                   10250,
	MakeIPTablesUtilChains: utils.BoolPtr(true),
	HealthzBindAddress:     "0.0.0.0",
	HealthzPort:            utils.Int32Ptr(10248),
	ClusterDomain:          "cluster.local",
	EventRecordQPS:         utils.Int32Ptr(0),
	FailSwapOn:             utils.BoolPtr(false),
	ReadOnlyPort:           utils.Int32Ptr(0),
	TLSCertFile:            "/var/state/apiserver.cert",
	TLSPrivateKeyFile:      "/var/state/apiserver.key",
	StreamingConnectionIdleTimeout: metav1.Duration{
		Duration: 5 * time.Minute,
	},
	EvictionHard: map[string]string{
		"nodefs.available":   "5%",
		"imagefs.available":  "5%",
		"nodefs.inodesFree":  "5%",
		"imagefs.inodesFree": "5%",
	},
	EvictionSoft: map[string]string{
		"nodefs.available":   "10%",
		"imagefs.available":  "10%",
		"nodefs.inodesFree":  "10%",
		"imagefs.inodesFree": "10%",
	},
	EvictionSoftGracePeriod: map[string]string{
		"nodefs.available":   "1h",
		"imagefs.available":  "1h",
		"nodefs.inodesFree":  "1h",
		"imagefs.inodesFree": "1h",
	},
	Authentication: component.KubeletAuthentication{
		Webhook: component.KubeletWebhookAuthentication{
			Enabled: utils.BoolPtr(true),
		},
		Anonymous: component.KubeletAnonymousAuthentication{
			Enabled: utils.BoolPtr(true),
		},
		X509: component.KubeletX509Authentication{
			ClientCAFile: "/var/state/root.cert",
		},
	},
	Authorization: component.KubeletAuthorization{
		Mode: component.KubeletAuthorizationModeWebhook,
	},
}

// KubeletConfigOverrides specifies the subset of kubelet configuration that cannot be overridden
// externally - i.e. will always have the specified default
var KubeletConfigOverrides = component.KubeletConfiguration{
	TypeMeta: metav1.TypeMeta{
		Kind:       "KubeletConfiguration",
		APIVersion: "kubelet.config.k8s.io/v1beta1",
	},
	Address:                "0.0.0.0",
	Port:                   10250,
	MakeIPTablesUtilChains: utils.BoolPtr(true),
	HealthzBindAddress:     "0.0.0.0",
	HealthzPort:            utils.Int32Ptr(10248),
	ClusterDomain:          "cluster.local",
	ReadOnlyPort:           utils.Int32Ptr(0),
	TLSCertFile:            "/var/state/apiserver.cert",
	TLSPrivateKeyFile:      "/var/state/apiserver.key",
	Authentication: component.KubeletAuthentication{
		Webhook: component.KubeletWebhookAuthentication{
			Enabled: utils.BoolPtr(true),
		},
		Anonymous: component.KubeletAnonymousAuthentication{
			Enabled: utils.BoolPtr(true),
		},
		X509: component.KubeletX509Authentication{
			ClientCAFile: "/var/state/root.cert",
		},
	},
	Authorization: component.KubeletAuthorization{
		Mode: component.KubeletAuthorizationModeWebhook,
	},
}

var allCaps = []string{
	"CAP_AUDIT_CONTROL",
	"CAP_AUDIT_WRITE",
	"CAP_BLOCK_SUSPEND",
	"CAP_CHOWN",
	"CAP_DAC_OVERRIDE",
	"CAP_DAC_READ_SEARCH",
	"CAP_FOWNER",
	"CAP_FSETID",
	"CAP_IPC_LOCK",
	"CAP_IPC_OWNER",
	"CAP_KILL",
	"CAP_LEASE",
	"CAP_LINUX_IMMUTABLE",
	"CAP_MAC_ADMIN",
	"CAP_MAC_OVERRIDE",
	"CAP_MKNOD",
	"CAP_NET_ADMIN",
	"CAP_NET_BIND_SERVICE",
	"CAP_NET_BROADCAST",
	"CAP_NET_RAW",
	"CAP_SETGID",
	"CAP_SETFCAP",
	"CAP_SETPCAP",
	"CAP_SETUID",
	"CAP_SYS_ADMIN",
	"CAP_SYS_BOOT",
	"CAP_SYS_CHROOT",
	"CAP_SYS_MODULE",
	"CAP_SYS_NICE",
	"CAP_SYS_PACCT",
	"CAP_SYS_PTRACE",
	"CAP_SYS_RAWIO",
	"CAP_SYS_RESOURCE",
	"CAP_SYS_TIME",
	"CAP_SYS_TTY_CONFIG",
	"CAP_SYSLOG",
	"CAP_WAKE_ALARM",
}
