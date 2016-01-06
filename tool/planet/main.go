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

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/orbit/lib/utils"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/version"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/opencontainers/runc/libcontainer"
	"github.com/gravitational/planet/Godeps/_workspace/src/gopkg.in/alecthomas/kingpin.v2"
	"github.com/gravitational/planet/lib/agent"
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
	args, extraArgs := utils.SplitAt(os.Args, "--")

	var (
		app           = kingpin.New("planet", "Planet is a Kubernetes delivered as an orbit container")
		debug         = app.Flag("debug", "Enable debug mode").Bool()
		fromContainer = app.Flag("from-container", "Specifies if a command is run in container context").Bool()
		socketPath    = app.Flag("socket-path", "Path to the socket file").Default("/var/run/planet.socket").String()
		cversion      = app.Command("version", "Print version information")

		// internal init command used by libcontainer
		cinit = app.Command("init", "Internal init command").Hidden()

		// start the container with planet
		cstart = app.Command("start", "Start Planet container")

		cstartPublicIP           = cstart.Flag("public-ip", "IP accessible by other nodes for inter-host communication").OverrideDefaultFromEnvar("PLANET_PUBLIC_IP").IP()
		cstartMasterIP           = cstart.Flag("master-ip", "IP of the master POD (defaults to public-ip)").OverrideDefaultFromEnvar("PLANET_MASTER_IP").IP()
		cstartCloudProvider      = cstart.Flag("cloud-provider", "cloud provider name, e.g. 'aws' or 'gce'").OverrideDefaultFromEnvar("PLANET_CLOUD_PROVIDER").String()
		cstartClusterID          = cstart.Flag("cluster-id", "id of the cluster").OverrideDefaultFromEnvar("PLANET_CLUSTER_ID").String()
		cstartIgnoreChecks       = cstart.Flag("ignore-checks", "Force start ignoring some failed host checks (e.g. kernel version)").OverrideDefaultFromEnvar("PLANET_FORCE").Bool()
		cstartEnv                = EnvVars(cstart.Flag("env", "Set environment variable").OverrideDefaultFromEnvar("PLANET_ENV"))
		cstartMounts             = Mounts(cstart.Flag("volume", "External volume to mount").OverrideDefaultFromEnvar("PLANET_VOLUME"))
		cstartRoles              = List(cstart.Flag("role", "Roles such as 'master' or 'node'").OverrideDefaultFromEnvar("PLANET_ROLE"))
		cstartInsecureRegistries = List(cstart.Flag("insecure-registry", "Optional insecure registries").OverrideDefaultFromEnvar("PLANET_INSECURE_REGISTRY"))
		cstartStateDir           = cstart.Flag("state-dir", "directory where planet-specific state like keys and certificates is stored").Default("/var/planet/state").OverrideDefaultFromEnvar("PLANET_STATE_DIR").String()
		cstartServiceSubnet      = CIDRFlag(cstart.Flag("service-subnet", "subnet dedicated to the services in cluster").Default("10.100.0.0/16").OverrideDefaultFromEnvar("PLANET_SERVICE_SUBNET"))
		cstartPODSubnet          = CIDRFlag(cstart.Flag("pod-subnet", "subnet dedicated to the pods in the cluster").Default("10.244.0.0/16").OverrideDefaultFromEnvar("PLANET_POD_SUBNET"))
		cstartSelfTest           = cstart.Flag("self-test", "Run end-to-end tests on the started cluster").Bool()
		cstartTestSpec           = cstart.Flag("test-spec", "Regexp of the test specs to run (self-test mode only)").Default("Networking|Pods").String()
		cstartTestKubeRepoPath   = cstart.Flag("repo-path", "Path to either a k8s repository or a directory with test configuration files (self-test mode only)").String()

		// stop a running container
		cstop = app.Command("stop", "Stop planet container")

		// enter a running container
		center      = app.Command("enter", "Enter running planet container")
		centerArgs  = center.Arg("cmd", "command to execute").Default("/bin/bash").String()
		centerNoTTY = center.Flag("notty", "do not attach TTY to this process").Bool()
		centerUser  = center.Flag("user", "user to execute the command").Default("root").String()

		// planet agent mode
		cagent = app.Command("agent", "Run planet agent")
		// FIXME: wrap as HostPort
		cagentBindAddr = cagent.Flag("bind-addr", "address to bind network listeners to.  To use an IPv6 address, specify [::1] or [::1]:7946.").Default("0.0.0.0:7946").String()
		cagentRPCAddr  = cagent.Flag("rpc-addr", "Address to bind the RPC listener").Default("127.0.0.1:7373").String()
		cagentKubeAddr = cagent.Flag("kube-addr", "Address of the kubernetes api server").Default("127.0.0.1:8080").String()
		cagentJoin     = cagent.Flag("join", "Address of the agent to join").String()
		cagentRole     = cagent.Flag("role", "Agent operating role (master/node)").Default("master").String()
		cagentName     = cagent.Flag("name", "Agent name").String()

		// report status of the cluster
		cstatus        = app.Command("status", "Query the planet cluster status")
		cstatusRPCAddr = cstatus.Flag("rpc-addr", "agent RPC address").Default("127.0.0.1:7373").String()

		// test command
		ctest             = app.Command("test", "Run end-to-end tests on a running cluster")
		ctestKubeAddr     = HostPort(ctest.Flag("kube-addr", "Address of the kubernetes api server").Required())
		ctestKubeRepoPath = ctest.Flag("kube-repo", "Path to a kubernetes repository").String()
		ctestAssetPath    = ctest.Flag("asset-dir", "Path to test executables and data files").String()
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

	case cagent.FullCommand():
		if *cagentName == "" {
			*cagentName, err = os.Hostname()
			if err != nil {
				break
			}
		}
		conf := &agent.Config{
			Name:     *cagentName,
			BindAddr: *cagentBindAddr,
			RPCAddr:  *cagentRPCAddr,
		}
		monitoringConf := &monitoring.Config{
			Role:     monitoring.Role(*cagentRole),
			KubeAddr: *cagentKubeAddr,
			// MasterIP: *cstartMasterIP,
			// ClusterIP: clusterIP(*cstartServiceSubnet),
		}
		err = runAgent(conf, monitoringConf, *cagentJoin)

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
		config := Config{
			Rootfs:             rootfs,
			SocketPath:         *socketPath,
			Env:                *cstartEnv,
			Mounts:             *cstartMounts,
			IgnoreChecks:       *cstartIgnoreChecks,
			Roles:              *cstartRoles,
			InsecureRegistries: *cstartInsecureRegistries,
			MasterIP:           cstartMasterIP.String(),
			PublicIP:           cstartPublicIP.String(),
			CloudProvider:      *cstartCloudProvider,
			ClusterID:          *cstartClusterID,
			StateDir:           *cstartStateDir,
			ServiceSubnet:      *cstartServiceSubnet,
			PODSubnet:          *cstartPODSubnet,
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
		if *fromContainer {
			var ok bool
			ok, err = clusterStatus(*cstatusRPCAddr)
			if err == nil && !ok {
				err = &box.ExitError{Code: 1}
			}
		} else {
			rootfs, err = findRootfs()
			if err != nil {
				break
			}
			err = status(rootfs, *socketPath, *cstatusRPCAddr)
		}

	// "test" command
	case ctest.FullCommand():
		config := &e2e.Config{
			KubeMasterAddr: ctestKubeAddr.String(),
			KubeRepoPath:   *ctestKubeRepoPath,
			AssetDir:       *ctestAssetPath,
		}
		err = e2e.RunTests(config, extraArgs)
	default:
		err = trace.Errorf("unsupported command: %v", cmd)
	}

	return err
}

func selfTest(config Config, repoDir, spec string, extraArgs []string) error {
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
				err = trace.Wrap(fmt.Errorf("cannot start testing: cluster not running"))
			}
		case <-time.After(idleTimeout):
			err = trace.Wrap(fmt.Errorf("timed out waiting for units to come up"))
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
