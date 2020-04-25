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
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log/syslog"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gravitational/planet/lib/box"
	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/monitoring"
	"github.com/gravitational/planet/test/e2e"

	"github.com/fatih/color"
	kv "github.com/gravitational/configure"
	"github.com/gravitational/configure/cstrings"
	etcdconf "github.com/gravitational/coordinate/config"
	"github.com/gravitational/satellite/agent"
	"github.com/gravitational/satellite/agent/backend/inmemory"
	"github.com/gravitational/satellite/lib/history/sqlite"
	"github.com/gravitational/trace"
	"github.com/gravitational/version"
	serf "github.com/hashicorp/serf/client"
	"github.com/opencontainers/runc/libcontainer/configs"
	log "github.com/sirupsen/logrus"
	logsyslog "github.com/sirupsen/logrus/hooks/syslog"
	"gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	initLogging(false)
	var err error
	// Workaround the issue described here:
	// https://github.com/kubernetes/kubernetes/issues/17162
	_ = flag.CommandLine.Parse([]string{})

	if err = run(); err == nil {
		return
	}
	if errExit, ok := trace.Unwrap(err).(*box.ExitError); ok {
		os.Exit(errExit.Code)
	}
	die(err)
}

func run() error {
	var (
		app             = kingpin.New("planet", "Planet is a Kubernetes delivered as RunC container")
		debug           = app.Flag("debug", "Enable debug mode").Bool()
		socketPath      = app.Flag("socket-path", "Path to the socket file").Default("/var/run/planet.socket").String()
		profileEndpoint = app.Flag("httpprofile", "enable profiling endpoint on specified host/port i.e. localhost:7070").Hidden().String()

		// commands
		cversion = app.Command("version", "Print version information")

		// internal init command used by libcontainer
		cinit = app.Command("init", "Internal init command").Hidden()

		// start the container with planet
		cstart = app.Command("start", "Start Planet container")

		cstartPublicIP       = cstart.Flag("public-ip", "IP accessible by other nodes for inter-host communication").OverrideDefaultFromEnvar("PLANET_PUBLIC_IP").IP()
		cstartMasterIP       = cstart.Flag("master-ip", "IP of the master Pod (defaults to public-ip)").OverrideDefaultFromEnvar("PLANET_MASTER_IP").IP()
		cstartCloudProvider  = cstart.Flag("cloud-provider", "cloud provider name, e.g. 'aws' or 'gce'").OverrideDefaultFromEnvar("PLANET_CLOUD_PROVIDER").String()
		cstartClusterID      = cstart.Flag("cluster-id", "ID of the cluster").OverrideDefaultFromEnvar("PLANET_CLUSTER_ID").String()
		cstartGCENodeTags    = cstart.Flag("gce-node-tags", "Node tag to set in the cloud configuration file on GCE as comma-separated values").OverrideDefaultFromEnvar(EnvGCENodeTags).String()
		cstartIgnoreChecks   = cstart.Flag("ignore-checks", "Force start ignoring some failed host checks (e.g. kernel version)").OverrideDefaultFromEnvar("PLANET_FORCE").Bool()
		cstartEnv            = EnvVars(cstart.Flag("env", "Set environment variable as comma-separated list of name=value pairs").OverrideDefaultFromEnvar("PLANET_ENV"))
		cstartMounts         = Mounts(cstart.Flag("volume", "External volume to mount, as a src:dst[:options] tuple").OverrideDefaultFromEnvar("PLANET_VOLUME"))
		cstartDevices        = Devices(cstart.Flag("device", "Device to create inside container").OverrideDefaultFromEnvar("PLANET_DEVICE"))
		cstartRoles          = List(cstart.Flag("role", "Roles such as 'master' or 'node'").OverrideDefaultFromEnvar("PLANET_ROLE"))
		cstartSecretsDir     = cstart.Flag("secrets-dir", "Directory with master secrets - certificate authority and certificates").OverrideDefaultFromEnvar("PLANET_SECRETS_DIR").ExistingDir()
		cstartServiceCIDR    = kv.CIDRFlag(cstart.Flag("service-subnet", "IP range from which to assign service cluster IPs. This must not overlap with any IP ranges assigned to nodes for pods.").Default(DefaultServiceSubnet).OverrideDefaultFromEnvar("PLANET_SERVICE_SUBNET"))
		cstartPodCIDR        = kv.CIDRFlag(cstart.Flag("pod-subnet", "subnet dedicated to the pods in the cluster").Default(DefaultPodSubnet).OverrideDefaultFromEnvar("PLANET_POD_SUBNET"))
		cstartProxyPortRange = cstart.Flag("proxy-portrange", "Range of host ports (beginPort-endPort, single port or beginPort+offset, inclusive) that may be consumed in order to proxy service traffic. If (unspecified, 0, or 0-0) then ports will be randomly chosen.").
					OverrideDefaultFromEnvar(EnvPlanetProxyPortRange).String()
		cstartServiceNodePortRange = cstart.Flag("service-node-portrange", "A port range to reserve for services with NodePort visibility. Example: '30000-32767'. Inclusive at both ends of the range.").
						Default(DefaultServiceNodePortRange).
						OverrideDefaultFromEnvar(EnvPlanetServiceNodePortRange).
						String()
		cstartFeatureGates = cstart.Flag("feature-gates", "A comma-separated list of key=value pairs that describe feature gates for alpha/experimental features.").
					Default(DefaultFeatureGates).
					OverrideDefaultFromEnvar(EnvPlanetFeatureGates).
					String()
		cstartVxlanPort               = cstart.Flag("vxlan-port", "overlay network port").Default(strconv.Itoa(DefaultVxlanPort)).OverrideDefaultFromEnvar(EnvVxlanPort).Int()
		cstartServiceUID              = cstart.Flag("service-uid", "service user ID. Service user is used for services that do not require elevated permissions").OverrideDefaultFromEnvar(EnvServiceUID).String()
		cstartSelfTest                = cstart.Flag("self-test", "Run end-to-end tests on the started cluster").Bool()
		cstartTestSpec                = cstart.Flag("test-spec", "Regexp of the test specs to run (self-test mode only)").Default("Networking|Pods").String()
		cstartTestKubeRepoPath        = cstart.Flag("repo-path", "Path to either a k8s repository or a directory with test configuration files (self-test mode only)").String()
		cstartEtcdProxy               = cstart.Flag("etcd-proxy", "Etcd proxy mode: 'off', 'on' or 'readonly'").OverrideDefaultFromEnvar("PLANET_ETCD_PROXY").String()
		cstartEtcdMemberName          = cstart.Flag("etcd-member-name", "Etcd member name").OverrideDefaultFromEnvar("PLANET_ETCD_MEMBER_NAME").String()
		cstartEtcdInitialCluster      = KeyValueList(cstart.Flag("etcd-initial-cluster", "Initial etcd cluster configuration (list of peers)").OverrideDefaultFromEnvar("PLANET_ETCD_INITIAL_CLUSTER"))
		cstartEtcdInitialClusterState = cstart.Flag("etcd-initial-cluster-state", "Etcd initial cluster state: 'new' or 'existing'").OverrideDefaultFromEnvar("PLANET_ETCD_INITIAL_CLUSTER_STATE").String()
		cstartEtcdOptions             = cstart.Flag("etcd-options", "Additional command line options to pass to etcd").OverrideDefaultFromEnvar("PLANET_ETCD_OPTIONS").String()
		cstartInitialCluster          = KeyValueList(cstart.Flag("initial-cluster", "Initial planet cluster configuration as a comma-separated list of peers").OverrideDefaultFromEnvar(EnvInitialCluster))
		cstartNodeName                = cstart.Flag("node-name", "Identify the node with this string instead of hostname in kubernetes services").OverrideDefaultFromEnvar("PLANET_NODE_NAME").String()
		cstartHostname                = cstart.Flag("hostname", "Hostname to set inside container").OverrideDefaultFromEnvar("PLANET_HOSTNAME").String()
		// Docker options
		cstartDockerOptions   = cstart.Flag("docker-options", "Additional options to pass to docker daemon").OverrideDefaultFromEnvar("PLANET_DOCKER_OPTIONS").String()
		cstartDockerBackend   = cstart.Flag("docker-backend", "Docker backend to use. If no backend has been specified, one is selected automatically.").OverrideDefaultFromEnvar("PLANET_DOCKER_BACKEND").String()
		cstartElectionEnabled = Bool(cstart.Flag("election-enabled", "Boolean flag to control if the agent initially starts with election participation on").OverrideDefaultFromEnvar(EnvElectionEnabled))
		cstartDNSHosts        = DNSOverrides(cstart.Flag("dns-hosts", "Comma-separated list of domain name to IP address mappings as 'domain/ip' pairs").OverrideDefaultFromEnvar(EnvDNSHosts))
		cstartDNSZones        = DNSOverrides(cstart.Flag("dns-zones", "Comma-separated list of DNS zone to nameserver IP mappings as 'zone/nameserver' pairs").OverrideDefaultFromEnvar(EnvDNSZones))
		cstartKubeletOptions  = cstart.Flag("kubelet-options", "Additional command line options to pass to kubelet").
					OverrideDefaultFromEnvar(EnvPlanetKubeletOptions).String()
		cstartAPIServerOptions = cstart.Flag("apiserver-options", "Additional command line options to pass to API server").
					OverrideDefaultFromEnvar(EnvPlanetAPIServerOptions).String()
		cstartDNSListenAddrs = List(cstart.Flag("dns-listen-addr", "Comma-separated list of addresses for CoreDNS to listen on").OverrideDefaultFromEnvar(EnvPlanetDNSListenAddr).Default(DefaultDNSListenAddr))
		cstartDNSPort        = cstart.Flag("dns-port", "DNS port for CoreDNS").OverrideDefaultFromEnvar(EnvPlanetDNSPort).Default(strconv.Itoa(DNSPort)).Int()
		cstartTaints         = List(cstart.Flag("taint", "Kubernetes taints to apply to the node during creation").OverrideDefaultFromEnvar(EnvPlanetTaints))
		cstartNodeLabels     = List(cstart.Flag("node-label", "Kubernetes node label to apply upon node registration").OverrideDefaultFromEnvar(EnvPlanetNodeLabels))
		cstartDisableFlannel = cstart.Flag("disable-flannel", "Disable flannel within the planet container").OverrideDefaultFromEnvar(EnvDisableFlannel).Bool()
		cstartKubeletConfig  = cstart.Flag("kubelet-config", "Kubelet configuration as base64-encoded JSON payload").OverrideDefaultFromEnvar(EnvPlanetKubeletConfig).String()
		cstartCloudConfig    = cstart.Flag("cloud-config", "Cloud configuration as base64-encoded payload").OverrideDefaultFromEnvar(EnvPlanetCloudConfig).String()

		// start the planet agent
		cagent                 = app.Command("agent", "Start Planet Agent")
		cagentPublicIP         = cagent.Flag("public-ip", "IP accessible by other nodes for inter-host communication").OverrideDefaultFromEnvar(EnvPublicIP).IP()
		cagentLeaderKey        = cagent.Flag("leader-key", "Etcd key holding the new leader").Required().String()
		cagentElectionKey      = cagent.Flag("election-key", "Etcd key to control if the current node is participating in leader election. Contains list of IPs of nodes currently participating in election. To have a node stop participating in election, remove its IP from this list.").Required().String()
		cagentRole             = cagent.Flag("role", "Server role").OverrideDefaultFromEnvar(EnvRole).String()
		cagentKubeAPIServerDNS = cagent.Flag("apiserver-dns", "Kubernetes API server DNS entry").OverrideDefaultFromEnvar(EnvAPIServerName).String()
		cagentTerm             = cagent.Flag("term", "Leader lease duration").Default(DefaultLeaderTerm.String()).Duration()
		cagentRPCAddrs         = List(cagent.Flag("rpc-addr", "Address to bind the RPC listener to.  Can be specified multiple times").Default("127.0.0.1:7575"))
		cagentMetricsAddr      = cagent.Flag("metrics-addr", "Address to listen on for web interface and telemetry for Prometheus metrics").Default("127.0.0.1:7580").String()
		cagentKubeAddr         = cagent.Flag("kube-addr", "Address of the kubernetes API server.  Will default to apiserver-dns:8080").String()
		cagentName             = cagent.Flag("name", "Agent name.  Must be the same as the name of the local serf node").OverrideDefaultFromEnvar(EnvAgentName).String()
		cagentNodeName         = cagent.Flag("node-name", "Kubernetes node name").OverrideDefaultFromEnvar(EnvNodeName).String()
		cagentSerfRPCAddr      = cagent.Flag("serf-rpc-addr", "RPC address of the local serf node").Default("127.0.0.1:7373").String()
		cagentInitialCluster   = KeyValueList(cagent.Flag("initial-cluster", "Initial planet cluster configuration as a comma-separated list of peers").OverrideDefaultFromEnvar(EnvInitialCluster))
		cagentClusterDNS       = cagent.Flag("cluster-dns", "IP for a cluster DNS server.").OverrideDefaultFromEnvar(EnvClusterDNSIP).IP()
		cagentRegistryAddr     = cagent.Flag("docker-registry-addr",
			"Address of the private docker registry.  Will default to apiserver-dns:5000").String()
		cagentEtcdEndpoints          = List(cagent.Flag("etcd-endpoints", "List of comma-separated etcd endpoints").Default(DefaultEtcdEndpoints))
		cagentEtcdCAFile             = cagent.Flag("etcd-cafile", "Certificate Authority file used to secure etcd communication").String()
		cagentEtcdCertFile           = cagent.Flag("etcd-certfile", "TLS certificate file used to secure etcd communication").String()
		cagentEtcdKeyFile            = cagent.Flag("etcd-keyfile", "TLS key file used to secure etcd communication").String()
		cagentElectionEnabled        = Bool(cagent.Flag("election-enabled", "Boolean flag to control if the agent initially starts with election participation on").OverrideDefaultFromEnvar(EnvElectionEnabled))
		cagentDNSUpstreamNameservers = List(cagent.Flag("nameservers", "List of additional upstream nameservers to add to DNS configuration as a comma-separated list of IPs").OverrideDefaultFromEnvar(EnvDNSUpstreamNameservers))
		cagentDNSZones               = DNSOverrides(cagent.Flag("dns-zones", "Comma-separated list of DNS zone to nameserver IP mappings as 'zone/nameserver' pairs").OverrideDefaultFromEnvar(EnvDNSZones))
		cagentCloudProvider          = cagent.Flag("cloud-provider", "Which cloud provider backend the cluster is using").OverrideDefaultFromEnvar(EnvCloudProvider).String()
		cagentHighWatermark          = cagent.Flag("high-watermark", "Usage percentage of monitored directories and devicemapper which is considered degrading").Default(strconv.Itoa(HighWatermark)).Uint64()
		cagentHTTPTimeout            = cagent.Flag("http-timeout", "Timeout for HTTP requests, formatted as Go duration.").OverrideDefaultFromEnvar(EnvPlanetAgentHTTPTimeout).Default(constants.HTTPTimeout.String()).Duration()
		cagentTimelineDir            = cagent.Flag("timeline-dir", "Directory to be used for timeline storage").Default("/tmp/timeline").String()
		cagentRetention              = cagent.Flag("retention", "Window to retain timeline as a Go duration").Duration()

		// stop a running container
		cstop = app.Command("stop", "Stop planet container")

		// enter a running container, deprecated, so hide it
		center      = app.Command("enter", "[DEPRECATED] Enter running planet container").Hidden().Interspersed(false)
		centerNoTTY = center.Flag("notty", "Do not attach TTY to this process").Bool()
		centerUser  = center.Flag("user", "User to execute the command").Default("root").String()
		centerCmd   = center.Arg("cmd", "Command to execute").Default("/bin/bash").String()

		// exec into running container
		cexec      = app.Command("exec", "Run a command in a running container").Interspersed(false)
		cexecTTY   = cexec.Flag("tty", "Allocate a pseudo-TTY").Short('t').Bool()
		cexecStdin = cexec.Flag("interactive", "Keep stdin open").Short('i').Bool()
		cexecUser  = cexec.Flag("user", "User to execute the command with").String()
		cexecCmd   = cexec.Arg("command", "Command to execute").Required().String()
		cexecArgs  = cexec.Arg("arg", "Additional arguments to command").Strings()

		// report status of the cluster
		cstatus            = app.Command("status", "Query the planet cluster status")
		cstatusLocal       = cstatus.Flag("local", "Query the status of the local node").Bool()
		cstatusRPCPort     = cstatus.Flag("rpc-port", "Local agent RPC port.").Default("7575").Int()
		cstatusPrettyPrint = cstatus.Flag("pretty", "Pretty-print the output").Default("true").Bool()
		cstatusTimeout     = cstatus.Flag("timeout", "Status timeout").Default(AgentStatusTimeout.String()).Duration()
		cstatusCAFile      = cstatus.Flag("ca-file", "CA to authenticate server").
					Default(ClientRPCCAPath).OverrideDefaultFromEnvar(EnvPlanetAgentCAFile).String()
		cstatusClientCertFile = cstatus.Flag("client-cert-file", "mTLS client certificate file").
					Default(ClientRPCCertPath).OverrideDefaultFromEnvar(EnvPlanetAgentClientCertFile).String()
		cstatusClientKeyFile = cstatus.Flag("client-key-file", "mTLS client key file").
					Default(ClientRPCKeyPath).OverrideDefaultFromEnvar(EnvPlanetAgentClientKeyFile).String()

		cstatusDump        = app.Command("debug-status", "Dump Planet Agent debug internals")
		cstatusDumpProfile = cstatusDump.Flag("profile", "Name of the profile to dump").Default("goroutine").String()
		cstatusDumpRPCPort = cstatusDump.Flag("rpc-port", "Local agent RPC port.").Default("7575").Int()
		cstatusDumpCAFile  = cstatusDump.Flag("ca-file", "CA to authenticate server").
					Default(ClientRPCCAPath).OverrideDefaultFromEnvar(EnvPlanetAgentCAFile).String()
		cstatusDumpClientCertFile = cstatusDump.Flag("client-cert-file", "mTLS client certificate file").
						Default(ClientRPCCertPath).OverrideDefaultFromEnvar(EnvPlanetAgentClientCertFile).String()
		cstatusDumpClientKeyFile = cstatusDump.Flag("client-key-file", "mTLS client key file").
						Default(ClientRPCKeyPath).OverrideDefaultFromEnvar(EnvPlanetAgentClientKeyFile).String()

		// test command
		ctest             = app.Command("test", "Run end-to-end tests on a running cluster")
		ctestKubeAddr     = HostPort(ctest.Flag("kube-addr", "Address of the kubernetes api server").Required())
		ctestKubeRepoPath = ctest.Flag("kube-repo", "Path to a kubernetes repository").String()
		ctestAssetPath    = ctest.Flag("asset-dir", "Path to test executables and data files").String()

		// device management
		cdevice = app.Command("device", "Manage devices in container")

		cdeviceAdd     = cdevice.Command("add", "Add new device to container")
		cdeviceAddData = cdeviceAdd.Flag("data", "Device definition as seen on host").Required().String()

		cdeviceRemove     = cdevice.Command("remove", "Remove device from container")
		cdeviceRemoveNode = cdeviceRemove.Flag("node", "Device node to remove").Required().String()

		// etcd related commands
		cetcd = app.Command("etcd", "Commands related to etcd")

		cetcdInit = cetcd.Command("init", "Setup etcd to run the correct version").Hidden()

		cetcdBackup       = cetcd.Command("backup", "Backup the etcd datastore to a file")
		cetcdBackupFile   = cetcdBackup.Arg("file", "The file to store the backup. If unspecified - stdout will be used.").String()
		cetcdBackupPrefix = cetcdBackup.Flag("prefix", "Optional etcd prefix to backup (e.g. /gravity). Can be supplied multiple times").Default(ETCDBackupPrefix).Strings()

		cetcdDisable        = cetcd.Command("disable", "Disable etcd on this node")
		cetcdDisableUpgrade = cetcdDisable.Flag("upgrade", "disable the upgrade service").Bool()
		cetcdStopApiserver  = cetcdDisable.Flag("stop-api", "stops the kubernetes API service").Bool()

		cetcdEnable        = cetcd.Command("enable", "Enable etcd on this node")
		cetcdEnableUpgrade = cetcdEnable.Flag("upgrade", "enable the upgrade service").Bool()

		cetcdUpgrade  = cetcd.Command("upgrade", "Upgrade etcd to the latest version")
		cetcdRollback = cetcd.Command("rollback", "Rollback etcd to the previous release")

		cetcdRestore     = cetcd.Command("restore", "Restore etcd backup as part of the upgrade")
		cetcdRestoreFile = cetcdRestore.Arg("file", "A previously taken backup file to use during upgrade").Required().ExistingFile()

		cetcdWipe          = cetcd.Command("wipe", "Wipe out all local etcd data").Hidden()
		cetcdWipeConfirmed = cetcdWipe.Flag("confirm", "Auto-confirm the action").Bool()

		// leader election commands
		cleader              = app.Command("leader", "Leader election control")
		cleaderPublicIP      = cleader.Flag("public-ip", "IP accessible by other nodes for inter-host communication").OverrideDefaultFromEnvar(EnvPublicIP).IP()
		cleaderElectionKey   = cleader.Flag("election-key", "Etcd key that defines the state of election participation for this node").String()
		cleaderEtcdCAFile    = cleader.Flag("etcd-cafile", "Certificate Authority file used to secure etcd communication").String()
		cleaderEtcdCertFile  = cleader.Flag("etcd-certfile", "TLS certificate file used to secure etcd communication").String()
		cleaderEtcdKeyFile   = cleader.Flag("etcd-keyfile", "TLS key file used to secure etcd communication").String()
		cleaderEtcdEndpoints = List(cleader.Flag("etcd-endpoints", "List of comma-separated etcd endpoints").Default(DefaultEtcdEndpoints))
		cleaderPause         = cleader.Command("pause", "Pause leader election participation for this node")
		cleaderResume        = cleader.Command("resume", "Resume leader election participation for this node")
		cleaderView          = cleader.Command("view", "Display the IP address of the active master")
		cleaderViewKey       = cleaderView.Flag("leader-key", "Etcd key holding the new leader").Required().String()
	)

	args, extraArgs := cstrings.SplitAt(os.Args[1:], "--")
	cmd, err := app.Parse(args)
	if err != nil {
		return err
	}

	initLogging(*debug)

	if *profileEndpoint != "" {
		go func() {
			log.Error(http.ListenAndServe(*profileEndpoint, nil))
		}()
	}

	if emptyIP(cstartMasterIP) {
		cstartMasterIP = cstartPublicIP
	}

	var rootfs string
	switch cmd {

	// "version" command
	case cversion.FullCommand():
		version.Print()

	// "agent" command
	case cagent.FullCommand():
		cache := inmemory.New()
		if *cagentKubeAddr == "" {
			*cagentKubeAddr = "127.0.0.1:8080"
		}
		if *cagentRegistryAddr == "" {
			*cagentRegistryAddr = fmt.Sprintf("%v:5000", *cagentKubeAPIServerDNS)
		}
		log.Infof("Kubernetes API server: %v", *cagentKubeAddr)
		log.Infof("Private docker registry: %v", *cagentRegistryAddr)
		conf := &agent.Config{
			Name:        *cagentName,
			RPCAddrs:    *cagentRPCAddrs,
			SerfConfig:  serf.Config{Addr: *cagentSerfRPCAddr},
			MetricsAddr: *cagentMetricsAddr,
			Cache:       cache,
			CAFile:      *cagentEtcdCAFile,
			CertFile:    *cagentEtcdCertFile,
			KeyFile:     *cagentEtcdKeyFile,
			TimelineConfig: sqlite.Config{
				DBPath:            *cagentTimelineDir,
				RetentionDuration: *cagentRetention,
			},
		}
		etcdConf := etcdconf.Config{
			Endpoints: *cagentEtcdEndpoints,
			CAFile:    *cagentEtcdCAFile,
			CertFile:  *cagentEtcdCertFile,
			KeyFile:   *cagentEtcdKeyFile,
		}
		disableInterPodCheck := true
		// Leave the inter-pod communication test disabled.
		// Planet uses a custom networking plugin (with calico implementating the plugin).
		// The configuration is two-fold:
		//  * kubelet command line that specifies the use of custom networking plugin / static
		//    configuration files and additional binaries
		//  * daemonset with calico node support tools
		//  * one-time configuration job
		// When updating from non-networking environment to version with custom networking plugin,
		// the plugin is enabled by default. If the other configuration (in kubernetes environment)
		// has not happened yet, the system will be in crippled state for as long as network configuration
		// is not complete. Running networking tests at this time will only make matters worse.
		// TODO: find a way to disable the testing initially and be able to resume if need be.
		//
		// if *cagentInitialCluster != nil && len(*cagentInitialCluster) > 2 {
		// 	disableInterPodCheck = false
		// }
		monitoringConf := &monitoring.Config{
			Role:                  agent.Role(*cagentRole),
			AdvertiseIP:           cagentPublicIP.String(),
			KubeAddr:              *cagentKubeAddr,
			ClusterDNS:            cagentClusterDNS.String(),
			UpstreamNameservers:   *cagentDNSUpstreamNameservers,
			DNSZones:              (map[string][]string)(*cagentDNSZones),
			RegistryAddr:          fmt.Sprintf("https://%v", *cagentRegistryAddr),
			NettestContainerImage: fmt.Sprintf("%v/gcr.io/google_containers/nettest:1.8", *cagentRegistryAddr),
			ETCDConfig:            etcdConf,
			DisableInterPodCheck:  disableInterPodCheck,
			CloudProvider:         *cagentCloudProvider,
			HighWatermark:         uint(*cagentHighWatermark),
			NodeName:              *cagentNodeName,
			HTTPTimeout:           *cagentHTTPTimeout,
		}
		leaderConf := &LeaderConfig{
			PublicIP:        cagentPublicIP.String(),
			LeaderKey:       *cagentLeaderKey,
			Role:            *cagentRole,
			Term:            *cagentTerm,
			ETCD:            etcdConf,
			APIServerDNS:    *cagentKubeAPIServerDNS,
			ElectionKey:     fmt.Sprintf("%v/%v", *cagentElectionKey, cagentPublicIP.String()),
			ElectionEnabled: bool(*cagentElectionEnabled),
		}
		err = runAgent(conf, monitoringConf, leaderConf, toAddrList(*cagentInitialCluster))

	case cleaderPause.FullCommand(), cleaderResume.FullCommand():
		etcdConf := &etcdconf.Config{
			Endpoints: *cleaderEtcdEndpoints,
			CAFile:    *cleaderEtcdCAFile,
			CertFile:  *cleaderEtcdCertFile,
			KeyFile:   *cleaderEtcdKeyFile,
		}
		memberKey := fmt.Sprintf("%v/%v", *cleaderElectionKey, *cleaderPublicIP)
		if cmd == cleaderPause.FullCommand() {
			err = leaderPause(cleaderPublicIP.String(), memberKey, etcdConf)
		} else {
			err = leaderResume(cleaderPublicIP.String(), memberKey, etcdConf)
		}
	case cleaderView.FullCommand():
		etcdConf := &etcdconf.Config{
			Endpoints: *cleaderEtcdEndpoints,
			CAFile:    *cleaderEtcdCAFile,
			CertFile:  *cleaderEtcdCertFile,
			KeyFile:   *cleaderEtcdKeyFile,
		}
		err = leaderView(*cleaderViewKey, etcdConf)

	// "start" command
	case cstart.FullCommand():
		if emptyIP(cstartPublicIP) && os.Getpid() > 5 {
			err = trace.Errorf("public-ip is not set")
			break
		}
		rootfs, err = findRootfs()
		if err != nil {
			break
		}
		setupSignalHandlers(rootfs, *socketPath)
		initialCluster := *cstartEtcdInitialCluster
		if initialCluster == nil {
			initialCluster = *cstartInitialCluster
		}
		config := &Config{
			Rootfs:               rootfs,
			SocketPath:           *socketPath,
			Env:                  *cstartEnv,
			Mounts:               *cstartMounts,
			Devices:              *cstartDevices,
			IgnoreChecks:         *cstartIgnoreChecks,
			Roles:                *cstartRoles,
			MasterIP:             cstartMasterIP.String(),
			PublicIP:             cstartPublicIP.String(),
			CloudProvider:        *cstartCloudProvider,
			ClusterID:            *cstartClusterID,
			GCENodeTags:          *cstartGCENodeTags,
			SecretsDir:           *cstartSecretsDir,
			ServiceCIDR:          *cstartServiceCIDR,
			PodCIDR:              *cstartPodCIDR,
			ProxyPortRange:       *cstartProxyPortRange,
			ServiceNodePortRange: *cstartServiceNodePortRange,
			FeatureGates:         *cstartFeatureGates,
			VxlanPort:            *cstartVxlanPort,
			InitialCluster:       *cstartInitialCluster,
			ServiceUser: serviceUser{
				UID: *cstartServiceUID,
			},
			EtcdProxy:               *cstartEtcdProxy,
			EtcdMemberName:          *cstartEtcdMemberName,
			EtcdInitialCluster:      toEtcdPeerList(initialCluster),
			EtcdGatewayList:         toEtcdGatewayList(initialCluster),
			EtcdInitialClusterState: *cstartEtcdInitialClusterState,
			EtcdOptions:             *cstartEtcdOptions,
			NodeName:                *cstartNodeName,
			Hostname:                *cstartHostname,
			DockerBackend:           *cstartDockerBackend,
			DockerOptions:           *cstartDockerOptions,
			ElectionEnabled:         bool(*cstartElectionEnabled),
			DNS: DNS{
				Hosts:       *cstartDNSHosts,
				Zones:       *cstartDNSZones,
				ListenAddrs: *cstartDNSListenAddrs,
				Port:        *cstartDNSPort,
			},
			KubeletOptions:   *cstartKubeletOptions,
			APIServerOptions: *cstartAPIServerOptions,
			Taints:           *cstartTaints,
			NodeLabels:       *cstartNodeLabels,
			DisableFlannel:   *cstartDisableFlannel,
			KubeletConfig:    *cstartKubeletConfig,
			CloudConfig:      *cstartCloudConfig,
		}
		if *cstartSelfTest {
			err = selfTest(config, *cstartTestKubeRepoPath, *cstartTestSpec, extraArgs)
		} else {
			err = startAndWait(config)
		}

	// "init" command
	case cinit.FullCommand():
		err = box.Init()

	// "enter" command
	case center.FullCommand():
		rootfs, err = findRootfs()
		if err != nil {
			break
		}
		err = enterConsole(
			rootfs, *socketPath, *centerCmd, *centerUser, !*centerNoTTY, true, extraArgs)

	// "exec" command
	case cexec.FullCommand():
		rootfs, err = findRootfs()
		if err != nil {
			break
		}
		err = enterConsole(
			rootfs, *socketPath, *cexecCmd, *cexecUser, *cexecTTY, *cexecStdin, *cexecArgs)

	// "stop" command
	case cstop.FullCommand():
		rootfs, err = findRootfs()
		if err != nil {
			break
		}
		err = stop(rootfs, *socketPath)

	// "status" command
	case cstatus.FullCommand():
		var ok bool
		ok, err = status(statusConfig{
			rpcPort:        *cstatusRPCPort,
			local:          *cstatusLocal,
			prettyPrint:    *cstatusPrettyPrint,
			timeout:        *cstatusTimeout,
			caFile:         *cstatusCAFile,
			clientCertFile: *cstatusClientCertFile,
			clientKeyFile:  *cstatusClientKeyFile,
		})
		if err == nil && !ok {
			err = trace.Errorf("status degraded")
		}

	// "debug-status" command
	case cstatusDump.FullCommand():
		err = getAgentDebugProfile(debugConfig{
			profile:        *cstatusDumpProfile,
			rpcPort:        *cstatusDumpRPCPort,
			caFile:         *cstatusDumpCAFile,
			clientCertFile: *cstatusDumpClientCertFile,
			clientKeyFile:  *cstatusDumpClientKeyFile,
		})

	// "test" command
	case ctest.FullCommand():
		config := &e2e.Config{
			KubeMasterAddr: ctestKubeAddr.String(),
			KubeRepoPath:   *ctestKubeRepoPath,
			AssetDir:       *ctestAssetPath,
		}
		err = e2e.RunTests(config, extraArgs)

	case cdeviceAdd.FullCommand():
		var device configs.Device
		if err = json.Unmarshal([]byte(*cdeviceAddData), &device); err != nil {
			break
		}
		err = createDevice(&device)

	case cdeviceRemove.FullCommand():
		err = removeDevice(*cdeviceRemoveNode)

	case cetcdInit.FullCommand():
		err = etcdInit()

	case cetcdBackup.FullCommand():
		err = etcdBackup(*cetcdBackupFile, *cetcdBackupPrefix)

	case cetcdEnable.FullCommand():
		err = etcdEnable(*cetcdEnableUpgrade)

	case cetcdDisable.FullCommand():
		err = etcdDisable(*cetcdDisableUpgrade, *cetcdStopApiserver)

	case cetcdUpgrade.FullCommand():
		err = etcdUpgrade(false)

	case cetcdRollback.FullCommand():
		err = etcdUpgrade(true)

	case cetcdRestore.FullCommand():
		err = etcdRestore(*cetcdRestoreFile)

	case cetcdWipe.FullCommand():
		err = etcdWipe(*cetcdWipeConfirmed)

	default:
		err = trace.Errorf("unsupported command: %v", cmd)
	}

	return err
}

const monitoringDbFile = "monitoring.db"

func selfTest(config *Config, repoDir, spec string, extraArgs []string) error {
	var ctx *runtimeContext
	var err error
	const idleTimeout = 30 * time.Second

	testConfig := &e2e.Config{
		KubeMasterAddr: config.MasterIP + ":8080", // FIXME: get from configuration
		KubeRepoPath:   repoDir,
	}

	monitorc := make(chan bool, 1)
	ctx, err = start(config, monitorc)
	if err == nil {
		select {
		case clusterUp := <-monitorc:
			if clusterUp {
				if spec != "" {
					log.Infof("Testing: %s", spec)
					extraArgs = append(extraArgs, fmt.Sprintf("-focus=%s", spec))
				}
				err = e2e.RunTests(testConfig, extraArgs)
			} else {
				err = trace.Errorf("cannot start testing: cluster not running")
			}
		case <-time.After(idleTimeout):
			err = trace.Errorf("timed out waiting for units to come up")
		}
		stop(config.Rootfs, config.SocketPath)
		ctx.Close()
	}

	return err
}

func EnvVars(s kingpin.Settings) *box.EnvVars {
	vars := new(box.EnvVars)
	s.SetValue(vars)
	return vars
}

func Mounts(s kingpin.Settings) *box.Mounts {
	vars := new(box.Mounts)
	s.SetValue(vars)
	return vars
}

func Devices(s kingpin.Settings) *box.Devices {
	vars := new(box.Devices)
	s.SetValue(vars)
	return vars
}

// DNSOverrides returns a CLI flag for DNS host/zone overrides
func DNSOverrides(s kingpin.Settings) *box.DNSOverrides {
	vars := &box.DNSOverrides{}
	s.SetValue(vars)
	return vars
}

func List(s kingpin.Settings) *list {
	l := new(list)
	s.SetValue(l)
	return l
}

func Bool(s kingpin.Settings) *boolFlag {
	f := new(boolFlag)
	s.SetValue(f)
	return f
}

func KeyValueList(s kingpin.Settings) *kv.KeyVal {
	l := new(kv.KeyVal)
	s.SetValue(l)
	return l
}

func HostPort(s kingpin.Settings) *hostPort {
	result := new(hostPort)

	s.SetValue(result)
	return result
}

// findRootfs returns the full path of RootFS this executalbe is in
func findRootfs() (string, error) {
	const rootfsDir = "/rootfs/"
	// look at the absolute path of planet executable, find '/rootfs/' substring in it,
	// that's the absolute rootfs path we need to return
	pePath, err := filepath.Abs(os.Args[0])
	if err != nil {
		return "", trace.Wrap(err, "failed to determine executable path")
	}
	idx := strings.Index(pePath, rootfsDir)
	if idx < 0 {
		return "", trace.Errorf("this executable needs to be placed inside %s", rootfsDir)
	}
	rootfsAbs := pePath[:idx+len(rootfsDir)-1]
	if _, err = os.Stat(rootfsAbs); err != nil {
		return "", trace.Wrap(err, "invalid RootFS: '%v'", rootfsAbs)
	}
	log.Infof("Starting in RootFS: %v", rootfsAbs)
	return rootfsAbs, nil
}

// setupSignalHandlers sets up a handler to handle common unix process signal traps.
// Some signals are handled to avoid the default handling which might be termination (SIGPIPE, SIGHUP, etc)
// The rest are considered as termination signals and the handler initiates shutdown upon receiving
// such a signal.
func setupSignalHandlers(rootfs, socketPath string) {
	oneOf := func(list []os.Signal, sig os.Signal) bool {
		for _, signal := range list {
			if signal == sig {
				return true
			}
		}
		return false
	}
	var ignores = []os.Signal{syscall.SIGPIPE, syscall.SIGHUP, syscall.SIGUSR2, syscall.SIGALRM}
	var terminals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT}
	c := make(chan os.Signal, 1)
	go func() {
		for sig := range c {
			switch {
			case sig == syscall.SIGUSR1:
				debug = !debug
				switchLoggingToDebug(debug)
			case oneOf(ignores, sig):
				log.Debugf("Received a %s signal, ignoring...", sig)
			default:
				log.Infof("Received a %s signal, stopping...", sig)
				err := stop(rootfs, socketPath)
				if err != nil {
					log.WithError(err).Error("Failed to stop.")
				}
				return
			}
		}
	}()
	signal.Notify(c, append(ignores, terminals...)...)
}

func emptyIP(addr *net.IP) bool {
	return len(*addr) == 0
}

// toAddrList interprets each key/value as domain=addr and extracts
// just the address part.
func toAddrList(store kv.KeyVal) (addrs []string) {
	for _, addr := range store {
		addrs = append(addrs, addr)
	}
	return addrs
}

// toEctdPeerList interprets each key/value pair as domain=addr,
// decorates each in etcd peer format.
func toEtcdPeerList(list kv.KeyVal) (peers string) {
	var addrs []string
	for domain, addr := range list {
		addrs = append(addrs, fmt.Sprintf("%v=https://%v:2380", domain, addr))
	}
	return strings.Join(addrs, ",")
}

// toEtcdGatewayList interprets each key/value pair, and
// formats it as a list of endpoints the etcd gateway can
// proxy to
func toEtcdGatewayList(list kv.KeyVal) (peers string) {
	var addrs []string
	for _, addr := range list {
		addrs = append(addrs, fmt.Sprintf("%v:2379", addr))
	}
	return strings.Join(addrs, ",")
}

// InitLogger configures the global logger for a given purpose / verbosity level
func initLogging(debug bool) {
	level := log.WarnLevel
	trace.SetDebug(debug)
	if debug {
		level = log.DebugLevel
	}
	log.StandardLogger().SetHooks(make(log.LevelHooks))
	formatter := &trace.TextFormatter{DisableTimestamp: true}
	log.SetFormatter(formatter)
	log.SetLevel(level)
	hook, err := logsyslog.NewSyslogHook("", "", syslog.LOG_WARNING, "")
	if err != nil {
		// syslog not available
		log.SetOutput(os.Stderr)
		return
	}
	log.AddHook(hook)
	log.SetOutput(ioutil.Discard)
}

func switchLoggingToDebug(debug bool) {
	level := log.DebugLevel
	if !debug {
		level = log.WarnLevel
	}
	log.SetLevel(level)
	trace.SetDebug(debug)
}

// debug controls whether the process is running in debug mode
var debug bool

// die prints the error message in red to the console and exits with a non-zero exit code
func die(err error) {
	log.WithError(err).Warn("Failed to run.")
	color.Red("[ERROR]: %v\n", trace.UserMessage(err))
	os.Exit(255)
}
