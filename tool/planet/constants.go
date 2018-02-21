package main

import "time"

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
	EnvAWSAccessKey            = "AWS_ACCESS_KEY_ID"
	EnvAWSSecretKey            = "AWS_SECRET_ACCESS_KEY"
	EnvKubeConfig              = "KUBECONFIG"
	EnvDNSOverrides            = "PLANET_DNS_OVERRIDES"
	EnvHostname                = "PLANET_HOSTNAME"
	EnvDNSUpstreamNameservers  = "PLANET_DNS_UPSTREAM_NAMESERVERS"
	EnvDockerOptions           = "DOCKER_OPTS"
	EnvEtcdOptions             = "ETCD_OPTS"
	EnvKubeletOptions          = "KUBELET_OPTS"
	EnvPlanetAgentCertFile     = "PLANET_AGENT_CERTFILE"
	EnvDockerPromiscuousMode   = "PLANET_DOCKER_PROMISCUOUS_MODE"

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
	APIServerDNSName = "leader.telekube.local"

	// LegacyAPIServerDNSName defines the DNS entry name of the master node
	// as it used to be in previous versions.
	// This is to keep compatibility with images vendored to the old location.
	LegacyAPIServerDNSName = "apiserver"

	// TelekubeDomain is the domain for local telekube cluster
	TelekubeDomain = "telekube.local"

	// CloudProviderAWS defines the name of the AWS cloud provider used to
	// setup AWS integration in kubernetes
	CloudProviderAWS = "aws"

	// See resolv.conf(5) on a Linux machine
	//
	// DNSNdots defines the threshold for amount of dots that must appear in a name
	// before an initial absolute query will be made
	DNSNdots = 2
	// DNSTimeout is the amount time resolver will wait for response before retrying
	// the query with a different name server. Measured in seconds
	DNSTimeout = 1

	// LocalDNSIP is the IP of the local DNS server
	LocalDNSIP = "127.0.0.1"

	// ETCDServiceName names the service unit for etcd
	ETCDServiceName = "etcd.service"
	// APIServerServiceName names the service unit for k8s apiserver
	APIServerServiceName = "kube-apiserver.service"

	// PlanetResolv is planet local resolver
	PlanetResolv = "resolv.gravity.conf"

	// SharedFileMask is file mask for shared file
	SharedFileMask = 0644

	// SharedDirMask is a permissions mask for a shared directory
	SharedDirMask = 0755

	// SharedReadWriteMask is a mask for a shared file with read/write access for everyone
	SharedReadWriteMask = 0666

	// DNSMasqK8sConf is DNSMasq DNS server K8s config
	DNSMasqK8sConf = "/etc/dnsmasq.d/k8s.conf"

	// DNSMasqAPIServerConf is the dnsmasq configuration file for apiserver
	DNSMasqAPIServerConf = "/etc/dnsmasq.d/apiserver.conf"

	// HostsFile specifies the location of the hosts configuration file
	HostsFile = "/etc/hosts"

	// HostnameFile specifies the location of the hostname configuration file
	HostnameFile = "/etc/hostname"

	// RootUID is id of the root user
	RootUID = 0
	// RootGID is id of the root group
	RootGID = 0

	// UsersDatabase is a file where Linux accounts information is stored
	UsersDatabase = "/etc/passwd"
	// UsersExtraDatabase is an alternate Linux accounts file on systems
	// where /etc/passwd is unavailable (e.g. /etc is read-only on Ubuntu Core)
	UsersExtraDatabase = "/var/lib/extrausers/passwd"
	// GroupsDatabase is a file where Linux groups information is stored
	GroupsDatabase = "/etc/group"
	// GroupsExtraDatabase is an alternate Linux groups file on systems
	// where /etc/group is unavailable (e.g. /etc is read-only on Ubuntu Core)
	GroupsExtraDatabase = "/var/lib/extrausers/group"

	// AgentStatusTimeout specifies the default status query timeout
	AgentStatusTimeout = 5 * time.Second

	// ClientRPCCertPath specifies the path to the CA certificate for agent RPC
	ClientRPCCertPath = "/var/state/root.cert"

	// DefaultDockerBridge specifies the default name of the docker bridge
	DefaultDockerBridge = "docker0"

	// DefaultDockerUnit specifies the name of the docker service unit file
	DefaultDockerUnit = "docker.service"

	// DockerPromiscuousModeDropIn names the drop-in file with promiscuous mode configuration
	// for docker bridge
	DockerPromiscuousModeDropIn = "99-docker-promisc.conf"
)

// K8sSearchDomains are default k8s search domain settings
var K8sSearchDomains = []string{
	"svc.cluster.local",
	"default.svc.cluster.local",
	"kube-system.svc.cluster.local",
}
