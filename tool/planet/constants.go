package main

import "time"

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
	// EnvPODSubnet names the environment variable that specifies
	// the subnet CIDR for k8s pods
	EnvPODSubnet = "KUBE_POD_SUBNET"
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
	EnvEtcdProxy = "ETCD_PROXY"
	// EnvEtcdMemberName names the environment variable that specifies
	// the name of this node in the etcd cluster
	EnvEtcdMemberName = "ETCD_MEMBER_NAME"
	// EnvEtcdInitialClusterState names the environment variable that specifies
	// the initial etcd cluster configuration for bootstrapping
	EnvEtcdInitialCluster = "ETCD_INITIAL_CLUSTER"
	// EnvEtcdInitialClusterState names the environment variable that specifies
	// the initial etcd cluster state
	EnvEtcdInitialClusterState = "ETCD_INITIAL_CLUSTER_STATE"
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
	// EnvDNSOverrides names the environment variable that specifies additional
	// DNS adderss overrides for container's dnsmasq
	EnvDNSOverrides = "PLANET_DNS_OVERRIDES"
	// EnvHostname names the environment variable that specifies the new
	// hostname
	EnvHostname = "PLANET_HOSTNAME"
	// EnvDNSUpstreamNameservers names the environment variable that specifies
	// additional nameservers to add to the container's dnsmasq configuration
	EnvDNSUpstreamNameservers = "PLANET_DNS_UPSTREAM_NAMESERVERS"
	// EnvDockerOptions names the environment variable that specifies additional
	// command line options for docker
	EnvDockerOptions = "DOCKER_OPTS"
	// EnvEtcdOptions names the environment variable that specifies additional etcd
	// command line options
	EnvEtcdOptions = "ETCD_OPTS"
	// EnvKubeletOptions names the environment variable that specifies additional
	// kubelet command line options
	EnvKubeletOptions = "KUBELET_OPTS"
	// EnvPlanetAgentCertFile names the environment variable that specifies the location
	// of the agent certificate file
	EnvPlanetAgentCertFile = "PLANET_AGENT_CERTFILE"
	// EnvDockerPromiscuousMode names the environment variable that specifies the
	// promiscuous mode for docker
	EnvDockerPromiscuousMode = "PLANET_DOCKER_PROMISCUOUS_MODE"
	// EnvServiceGID names the environment variable that specifies the service user ID
	EnvServiceUID = "PLANET_SERVICE_UID"
	// EnvServiceGID names the environment variable that specifies the service group ID
	EnvServiceGID = "PLANET_SERVICE_GID"

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

	// DefaultSecretsMountDir specifies the default location for certificates
	// as mapped inside the container
	DefaultSecretsMountDir = "/var/state"
	// DefaultEtcdctlCertFile is the path to the etcd certificate file
	DefaultEtcdctlCertFile = DefaultSecretsMountDir + "/etcd.cert"
	// DefaultEtcdctlKeyFile is the path to the etcd certificate key file
	DefaultEtcdctlKeyFile = DefaultSecretsMountDir + "/etcd.key"
	// DefaultEtcdctlCAFile is the path to the etcd CA certificate file
	DefaultEtcdctlCAFile = DefaultSecretsMountDir + "/root.cert"

	// APIServerDNSName defines the DNS entry name of the master node
	APIServerDNSName = "apiserver"
	// LeaderDNSName is a name of a current leader server
	LeaderDNSName = "leader.telekube.local"
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

	// RootUID is the ID of the root user
	RootUID = 0
	// RootGID is the ID of the root group
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

	// MinKernelVersion specifies the minimum kernel version on the host
	MinKernelVersion = 310
	// DefaultServiceSubnet specifies the subnet CIDR used for k8s Services by default
	DefaultServiceSubnet = "10.100.0.0/16"
	// DefaultPODSubnet specifies the subnet CIDR used for k8s Pods by default
	DefaultPODSubnet = "10.244.0.0/16"

	// ServiceUser specifies the name of the service user as seen inside the container.
	// Service user inside the container will be mapped to an existing user (not necessarily
	// with the same name) on host
	ServiceUser string = "planet"
	// ServiceGroup specifies the name of the service group as seen inside the container.
	ServiceGroup string = "planet"
)

// K8sSearchDomains are default k8s search domain settings
var K8sSearchDomains = []string{
	"svc.cluster.local",
	"default.svc.cluster.local",
	"kube-system.svc.cluster.local",
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
