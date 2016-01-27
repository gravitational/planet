package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/configure/cstrings"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/version"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/opencontainers/runc/libcontainer"
	"github.com/gravitational/planet/Godeps/_workspace/src/gopkg.in/alecthomas/kingpin.v2"
	"github.com/gravitational/planet/lib/agent"
	"github.com/gravitational/planet/lib/agent/backend/sqlite"
	"github.com/gravitational/planet/lib/agent/cache"
	"github.com/gravitational/planet/lib/box"
	"github.com/gravitational/planet/lib/monitoring"
	"github.com/gravitational/planet/test/e2e"
)

func main() {
	var exitCode int
	var err error

	if err = run(); err != nil {
		log.Errorf("Failed to run: '%v'\n", err)
		if errExit, ok := err.(*box.ExitError); ok {
			exitCode = errExit.Code
		}
	}
	os.Exit(exitCode)
}

func run() error {
	args, extraArgs := cstrings.SplitAt(os.Args, "--")

	var (
		app        = kingpin.New("planet", "Planet is a Kubernetes delivered as an orbit container")
		debug      = app.Flag("debug", "Enable debug mode").Bool()
		socketPath = app.Flag("socket-path", "Path to the socket file").Default("/var/run/planet.socket").String()
		cversion   = app.Command("version", "Print version information")

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
		cstartStateDir                = cstart.Flag("state-dir", "Directory where Planet will store state").OverrideDefaultFromEnvar("PLANET_STATE_DIR").String()
		cstartServiceSubnet           = CIDRFlag(cstart.Flag("service-subnet", "subnet dedicated to the services in cluster").Default(DefaultServiceSubnet).OverrideDefaultFromEnvar("PLANET_SERVICE_SUBNET"))
		cstartPODSubnet               = CIDRFlag(cstart.Flag("pod-subnet", "subnet dedicated to the pods in the cluster").Default(DefaultPODSubnet).OverrideDefaultFromEnvar("PLANET_POD_SUBNET"))
		cstartServiceUID              = cstart.Flag("service-uid", "uid to use for services").Default("1000").String()
		cstartServiceGID              = cstart.Flag("service-gid", "gid to use for services (defaults to service-uid)").String()
		cstartSelfTest                = cstart.Flag("self-test", "Run end-to-end tests on the started cluster").Bool()
		cstartTestSpec                = cstart.Flag("test-spec", "Regexp of the test specs to run (self-test mode only)").Default("Networking|Pods").String()
		cstartTestKubeRepoPath        = cstart.Flag("repo-path", "Path to either a k8s repository or a directory with test configuration files (self-test mode only)").String()
		cstartEtcdMemberName          = cstart.Flag("etcd-member-name", "Etcd member name").OverrideDefaultFromEnvar("PLANET_ETCD_MEMBER_NAME").String()
		cstartEtcdInitialCluster      = cstart.Flag("etcd-initial-cluster", "Initial etcd cluster configuration (list of peers)").OverrideDefaultFromEnvar("PLANET_ETCD_INITIAL_CLUSTER").String()
		cstartEtcdInitialClusterState = cstart.Flag("etcd-initial-cluster-state", "Etcd initial cluster state: 'new' or 'existing'").OverrideDefaultFromEnvar("PLANET_ETCD_INITIAL_CLUSTER_STATE").String()
		cstartInitialCluster          = cstart.Flag("initial-cluster", "Initial planet cluster configuration as a comma-separated list of peers").OverrideDefaultFromEnvar(EnvInitialCluster).String()

		// start the planet agent
		cagent               = app.Command("agent", "Start Planet Agent")
		cagentPublicIP       = cagent.Flag("public-ip", "IP accessible by other nodes for inter-host communication").OverrideDefaultFromEnvar(EnvPublicIP).IP()
		cagentLeaderKey      = cagent.Flag("leader-key", "Etcd key holding the new leader").Required().String()
		cagentRole           = cagent.Flag("role", "Server role").OverrideDefaultFromEnvar(EnvRole).String()
		cagentAPI            = cagent.Flag("apiserver-dns", "API server DNS entry").OverrideDefaultFromEnvar(EnvAPIServerName).String()
		cagentTerm           = cagent.Flag("term", "Leader lease duration").Default(DefaultLeaderTerm.String()).Duration()
		cagentEtcdEndpoints  = List(cagent.Flag("etcd-endpoints", "Etcd endpoints").Default(DefaultEtcdEndpoints))
		cagentRPCAddrs       = List(cagent.Flag("rpc-addr", "Address to bind the RPC listener to.  Can be specified multiple times").Default("127.0.0.1:7575"))
		cagentKubeAddr       = cagent.Flag("kube-addr", "Address of the kubernetes api server").Default("127.0.0.1:8080").String()
		cagentName           = cagent.Flag("name", "Agent name.  Must be the same as the name of the local serf node").OverrideDefaultFromEnvar(EnvAgentName).String()
		cagentSerfRPCAddr    = cagent.Flag("serf-rpc-addr", "RPC address of the local serf node").Default("127.0.0.1:7373").String()
		cagentInitialCluster = InlineList(cagent.Flag("initial-cluster", "Initial planet cluster configuration as a comma-separated list of peers").OverrideDefaultFromEnvar(EnvInitialCluster))
		cagentStateDir       = cagent.Flag("state-dir", "Directory where agent-specific state like health stats is stored").Default("/var/planet/agent").String()
		cagentClusterDNS     = cagent.Flag("cluster-dns", "IP for a cluster DNS server.").OverrideDefaultFromEnvar(EnvClusterDNSIP).IP()

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

		// test command
		ctest             = app.Command("test", "Run end-to-end tests on a running cluster")
		ctestKubeAddr     = HostPort(ctest.Flag("kube-addr", "Address of the kubernetes api server").Required())
		ctestKubeRepoPath = ctest.Flag("kube-repo", "Path to a kubernetes repository").String()
		ctestAssetPath    = ctest.Flag("asset-dir", "Path to test executables and data files").String()

		// secrets subsystem helps to manage master secrets
		csecrets = app.Command("secrets", "Subsystem to control k8s master secrets")

		// csecretsInit will create directory with secrets
		csecretsInit              = csecrets.Command("init", "initialize directory with secrets")
		csecretsInitDir           = csecretsInit.Arg("dir", "directory where secrets will be placed").Required().String()
		csecretsInitDomain        = csecretsInit.Flag("domain", "domain name for the certificate").Required().String()
		csecretsInitServiceSubnet = CIDRFlag(csecretsInit.Flag("service-subnet", "subnet dedicated to the services in cluster").Default(DefaultServiceSubnet))
	)

	cmd, err := app.Parse(args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed parsing command line arguments: %s.\nTry planet --help\n", err.Error())
		return err
	}

	if *debug == true {
		log.Initialize("console", "INFO")
		log.Infof("Debug mode is ON")
	} else {
		log.Initialize("console", "WARN")
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
		var cache cache.Cache
		path := filepath.Join(*cagentStateDir, monitoringDbFile)
		cache, err = sqlite.New(path)
		if err != nil {
			err = trace.Wrap(err, "failed to create cache")
			break
		}
		conf := &agent.Config{
			Name:        *cagentName,
			RPCAddrs:    *cagentRPCAddrs,
			SerfRPCAddr: *cagentSerfRPCAddr,
			Cache:       cache,
		}
		monitoringConf := &monitoring.Config{
			Role:       agent.Role(*cagentRole),
			KubeAddr:   *cagentKubeAddr,
			ClusterDNS: cagentClusterDNS.String(),
		}
		leaderConf := &LeaderConfig{
			PublicIP:      cagentPublicIP.String(),
			LeaderKey:     *cagentLeaderKey,
			Role:          *cagentRole,
			Term:          *cagentTerm,
			EtcdEndpoints: *cagentEtcdEndpoints,
			APIServerDNS:  *cagentAPI,
		}
		err = runAgent(conf, monitoringConf, leaderConf, []string(*cagentInitialCluster))

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
		setupSignalHanlders(rootfs, *socketPath)
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
			StateDir:                *cstartStateDir,
			ServiceSubnet:           *cstartServiceSubnet,
			PODSubnet:               *cstartPODSubnet,
			InitialCluster:          *cstartInitialCluster,
			ServiceUID:              *cstartServiceUID,
			ServiceGID:              *cstartServiceGID,
			EtcdMemberName:          *cstartEtcdMemberName,
			EtcdInitialCluster:      *cstartEtcdInitialCluster,
			EtcdInitialClusterState: *cstartEtcdInitialClusterState,
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
		ok, err = status(*cstatusRPCPort, *cstatusLocal, *cstatusPrettyPrint)
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

	case csecretsInit.FullCommand():
		err = initSecrets(
			*csecretsInitDir, *csecretsInitDomain, *csecretsInitServiceSubnet)

	default:
		err = trace.Errorf("unsupported command: %v", cmd)
	}

	return err
}

const monitoringDbFile = "monitoring.db"

func selfTest(config *Config, repoDir, spec string, extraArgs []string) error {
	var process *box.Box
	var err error
	const idleTimeout = 30 * time.Second

	testConfig := &e2e.Config{
		KubeMasterAddr: config.MasterIP + ":8080", // FIXME: get from configuration
		KubeRepoPath:   repoDir,
	}

	monitorc := make(chan bool, 1)
	process, err = start(config, monitorc)
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
		process.Close()
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

func InlineList(s kingpin.Settings) *stringList {
	l := new(stringList)
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

// setupSignalHanlders sets up a handler to interrupt SIGTERM and SIGINT
// allowing for a graceful shutdown via executing "stop" command
func setupSignalHanlders(rootfs, socketPath string) {
	c := make(chan os.Signal, 1)
	go func() {
		sig := <-c
		log.Infof("received a signal %v. stopping...\n", sig)
		err := stop(rootfs, socketPath)
		if err != nil {
			log.Errorf("error: %v", err)
		}
	}()
	signal.Notify(c, os.Interrupt, os.Kill, syscall.SIGTERM)
}

func emptyIP(addr *net.IP) bool {
	return len(*addr) == 0
}
