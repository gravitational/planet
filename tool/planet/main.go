package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gravitational/planet/lib/box"
	"github.com/gravitational/planet/lib/monitoring"
	"github.com/gravitational/planet/test/e2e"

	kv "github.com/gravitational/configure"
	"github.com/gravitational/configure/cstrings"
	etcdconf "github.com/gravitational/coordinate/config"
	"github.com/gravitational/satellite/agent"
	"github.com/gravitational/satellite/agent/backend/inmemory"
	"github.com/gravitational/trace"
	"github.com/gravitational/version"
	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/configs"
	log "github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	var exitCode int
	var err error

	if err = run(); err != nil {
		log.Errorf("Failed to run: '%v'\n", trace.DebugReport(err))
		if errExit, ok := trace.Unwrap(err).(*box.ExitError); ok {
			exitCode = errExit.Code
		} else {
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

func run() error {
	args, extraArgs := cstrings.SplitAt(os.Args, "--")

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

		cstartPublicIP                = cstart.Flag("public-ip", "IP accessible by other nodes for inter-host communication").OverrideDefaultFromEnvar("PLANET_PUBLIC_IP").IP()
		cstartMasterIP                = cstart.Flag("master-ip", "IP of the master POD (defaults to public-ip)").OverrideDefaultFromEnvar("PLANET_MASTER_IP").IP()
		cstartCloudProvider           = cstart.Flag("cloud-provider", "cloud provider name, e.g. 'aws' or 'gce'").OverrideDefaultFromEnvar("PLANET_CLOUD_PROVIDER").String()
		cstartClusterID               = cstart.Flag("cluster-id", "ID of the cluster").OverrideDefaultFromEnvar("PLANET_CLUSTER_ID").String()
		cstartIgnoreChecks            = cstart.Flag("ignore-checks", "Force start ignoring some failed host checks (e.g. kernel version)").OverrideDefaultFromEnvar("PLANET_FORCE").Bool()
		cstartEnv                     = EnvVars(cstart.Flag("env", "Set environment variable").OverrideDefaultFromEnvar("PLANET_ENV"))
		cstartMounts                  = Mounts(cstart.Flag("volume", "External volume to mount").OverrideDefaultFromEnvar("PLANET_VOLUME"))
		cstartRoles                   = List(cstart.Flag("role", "Roles such as 'master' or 'node'").OverrideDefaultFromEnvar("PLANET_ROLE"))
		cstartInsecureRegistries      = List(cstart.Flag("insecure-registry", "Optional insecure registries").OverrideDefaultFromEnvar("PLANET_INSECURE_REGISTRY"))
		cstartSecretsDir              = cstart.Flag("secrets-dir", "Directory with master secrets - certificate authority and certificates").OverrideDefaultFromEnvar("PLANET_SECRETS_DIR").ExistingDir()
		cstartServiceSubnet           = kv.CIDRFlag(cstart.Flag("service-subnet", "subnet dedicated to the services in cluster").Default(DefaultServiceSubnet).OverrideDefaultFromEnvar("PLANET_SERVICE_SUBNET"))
		cstartPODSubnet               = kv.CIDRFlag(cstart.Flag("pod-subnet", "subnet dedicated to the pods in the cluster").Default(DefaultPODSubnet).OverrideDefaultFromEnvar("PLANET_POD_SUBNET"))
		cstartServiceUID              = cstart.Flag("service-uid", "uid to use for services").Default("1000").String()
		cstartServiceGID              = cstart.Flag("service-gid", "gid to use for services (defaults to service-uid)").String()
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
		cstartDockerOptions         = cstart.Flag("docker-options", "Additional options to pass to docker daemon").OverrideDefaultFromEnvar("PLANET_DOCKER_OPTIONS").String()
		cstartDockerBackend         = cstart.Flag("docker-backend", "Docker backend to use. If no backend has been specified, one is selected automatically.").OverrideDefaultFromEnvar("PLANET_DOCKER_BACKEND").String()
		cstartElectionEnabled       = Bool(cstart.Flag("election-enabled", "Boolean flag to control if the agent initially starts with election participation on").OverrideDefaultFromEnvar(EnvElectionEnabled))
		cstartDNSOverrides          = KeyValueList(cstart.Flag("dns-overrides", "Comma-separated list of domain name to IP address mappings as key:value pairs").OverrideDefaultFromEnvar(EnvDNSOverrides))
		cstartKubeletOptions        = cstart.Flag("kubelet-options", "Additional command line options to pass to kubelet").OverrideDefaultFromEnvar("PLANET_KUBELET_OPTIONS").String()
		cstartDockerPromiscuousMode = cstart.Flag("docker-promiscuous-mode", "Whether to put docker bridge into promiscuous mode").OverrideDefaultFromEnvar(EnvDockerPromiscuousMode).Bool()

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

		// stop a running container
		cstop = app.Command("stop", "Stop planet container")

		// enter a running container
		center      = app.Command("enter", "Enter running planet container")
		centerArgs  = center.Arg("cmd", "command to execute").Default("/bin/bash").String()
		centerNoTTY = center.Flag("notty", "do not attach TTY to this process").Bool()
		centerUser  = center.Flag("user", "user to execute the command").Default("root").String()

		// report status of the cluster
		cstatus            = app.Command("status", "Query the planet cluster status")
		cstatusLocal       = cstatus.Flag("local", "Query the status of the local node").Bool()
		cstatusRPCPort     = cstatus.Flag("rpc-port", "Local agent RPC port.").Default("7575").Int()
		cstatusPrettyPrint = cstatus.Flag("pretty", "Pretty-print the output").Default("false").Bool()
		cstatusTimeout     = cstatus.Flag("timeout", "Status timeout").Default(AgentStatusTimeout.String()).Duration()
		cstatusCertFile    = cstatus.Flag("cert-file", "Client CA certificate to use for RPC call").
					Default(ClientRPCCertPath).OverrideDefaultFromEnvar(EnvPlanetAgentCertFile).String()

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
		cetcd          = app.Command("etcd", "Commands related to etcd")
		cetcdEndpoints = List(cetcd.Flag("etcd-endpoints", "List of comma-separated etcd endpoints").Default(DefaultEtcdEndpoints))
		cetcdCAFile    = cetcd.Flag("etcd-cafile", "Certificate Authority file used to secure etcd communication").String()
		cetcdCertFile  = cetcd.Flag("etcd-certfile", "TLS certificate file used to secure etcd communication").String()
		cetcdKeyFile   = cetcd.Flag("etcd-keyfile", "TLS key file used to secure etcd communication").String()

		cetcdPromote                    = cetcd.Command("promote", "Promote etcd running in proxy mode to a full member")
		cetcdPromoteName                = cetcdPromote.Flag("name", "Member name, as output by 'member add' command").Required().String()
		cetcdPromoteInitialCluster      = cetcdPromote.Flag("initial-cluster", "Initial cluster, as output by 'member add' command").Required().String()
		cetcdPromoteInitialClusterState = cetcdPromote.Flag("initial-cluster-state", "Initial cluster state, as output by 'member add' command").Required().String()

		cetcdBackup       = cetcd.Command("backup", "Backup the etcd v2 data store").Hidden()
		cetcdBackupFile   = cetcdBackup.Flag("file", "The file to store the backup (v2 datastore only)").Required().String()
		cetcdBackupPrefix = cetcdBackup.Flag("prefix", "the etcd path prefix to backup (default /)").Default("/").String()

		cetcdRestore       = cetcd.Command("restore", "Restore the etcd v2 data store from backup").Hidden()
		cetcdRestoreFile   = cetcdRestore.Flag("file", "The file to store the backup (v2 datastore only)").Required().String()
		cetcdRestorePrefix = cetcdRestore.Flag("prefix", "the etcd path prefix to backup (default /)").Default("/").String()

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

	cmd, err := app.Parse(args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed parsing command line arguments: %s.\nTry planet --help\n", err.Error())
		return err
	}

	if *debug {
		log.SetOutput(os.Stderr)
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetOutput(os.Stderr)
		log.SetLevel(log.WarnLevel)
	}

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
			SerfRPCAddr: *cagentSerfRPCAddr,
			MetricsAddr: *cagentMetricsAddr,
			Cache:       cache,
			CAFile:      *cagentEtcdCAFile,
			CertFile:    *cagentEtcdCertFile,
			KeyFile:     *cagentEtcdKeyFile,
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
			KubeAddr:              *cagentKubeAddr,
			ClusterDNS:            cagentClusterDNS.String(),
			UpstreamNameservers:   *cagentDNSUpstreamNameservers,
			RegistryAddr:          fmt.Sprintf("https://%v", *cagentRegistryAddr),
			NettestContainerImage: fmt.Sprintf("%v/gcr.io/google_containers/nettest:1.8", *cagentRegistryAddr),
			ETCDConfig:            etcdConf,
			DisableInterPodCheck:  disableInterPodCheck,
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
			Rootfs:                  rootfs,
			SocketPath:              *socketPath,
			Env:                     *cstartEnv,
			Mounts:                  *cstartMounts,
			IgnoreChecks:            *cstartIgnoreChecks,
			Roles:                   *cstartRoles,
			InsecureRegistries:      *cstartInsecureRegistries,
			MasterIP:                cstartMasterIP.String(),
			PublicIP:                cstartPublicIP.String(),
			CloudProvider:           *cstartCloudProvider,
			ClusterID:               *cstartClusterID,
			SecretsDir:              *cstartSecretsDir,
			ServiceSubnet:           *cstartServiceSubnet,
			PODSubnet:               *cstartPODSubnet,
			InitialCluster:          *cstartInitialCluster,
			ServiceUID:              *cstartServiceUID,
			ServiceGID:              *cstartServiceGID,
			EtcdProxy:               *cstartEtcdProxy,
			EtcdMemberName:          *cstartEtcdMemberName,
			EtcdInitialCluster:      toEtcdPeerList(initialCluster),
			EtcdInitialClusterState: *cstartEtcdInitialClusterState,
			EtcdOptions:             *cstartEtcdOptions,
			NodeName:                *cstartNodeName,
			Hostname:                *cstartHostname,
			DockerBackend:           *cstartDockerBackend,
			DockerOptions:           *cstartDockerOptions,
			ElectionEnabled:         bool(*cstartElectionEnabled),
			DNSOverrides:            *cstartDNSOverrides,
			KubeletOptions:          *cstartKubeletOptions,
			DockerPromiscuousMode:   *cstartDockerPromiscuousMode,
		}
		if *cstartSelfTest {
			err = selfTest(config, *cstartTestKubeRepoPath, *cstartTestSpec, extraArgs)
		} else {
			err = startAndWait(config)
		}

	// "init" command
	case cinit.FullCommand():
		err = initLibcontainer()

	// "enter" command
	case center.FullCommand():
		rootfs, err = findRootfs()
		if err != nil {
			break
		}
		err = enterConsole(
			rootfs, *socketPath, *centerArgs, *centerUser, !*centerNoTTY, extraArgs)

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
		ok, err = status(*cstatusRPCPort, *cstatusLocal, *cstatusPrettyPrint, *cstatusTimeout, *cstatusCertFile)
		if err == nil && !ok {
			err = trace.Errorf("status degraded")
		}

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

	case cetcdPromote.FullCommand():
		err = etcdPromote(*cetcdPromoteName, *cetcdPromoteInitialCluster, *cetcdPromoteInitialClusterState)

	case cetcdBackup.FullCommand():
		err = etcdBackup(*cetcdBackupFile, *cetcdBackupPrefix)

	case cetcdRestore.FullCommand():
		err = etcdBackup(*cetcdRestoreFile, *cetcdRestorePrefix)

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

// initCmd is implicitly called by the libcontainer logic and is used to start
// a process in the new namespaces and cgroups
func initLibcontainer() error {
	runtime.GOMAXPROCS(1)
	runtime.LockOSThread()
	factory, _ := libcontainer.New("")
	if err := factory.StartInitialization(); err != nil {
		log.Fatalf("error: %v", err)
	}
	return trace.Errorf("not reached")
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

	var ignores = []os.Signal{syscall.SIGPIPE, syscall.SIGHUP, syscall.SIGUSR1, syscall.SIGUSR2, syscall.SIGALRM}
	var terminals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT}
	c := make(chan os.Signal, 1)
	go func() {
		for sig := range c {
			switch {
			case oneOf(ignores, sig):
				log.Debugf("received a %s signal, ignoring...", sig)
			default:
				log.Infof("received a %s signal, stopping...", sig)
				err := stop(rootfs, socketPath)
				if err != nil {
					log.Errorf("error: %v", err)
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
