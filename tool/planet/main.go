package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/docker/docker/pkg/term"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/opencontainers/runc/libcontainer"
	"github.com/gravitational/planet/Godeps/_workspace/src/gopkg.in/alecthomas/kingpin.v2"
	"github.com/gravitational/planet/lib/box"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "planet error: %v\n", err)
		os.Exit(-1)
	}
	log.Infof("planet: execution completed successfully")
}

func run() error {
	var (
		app   = kingpin.New("planet", "Planet is a Kubernetes delivered as an orbit container")
		debug = app.Flag("debug", "Enable debug mode").Bool()

		// internal init command used by libcontainer
		cinit = app.Command("init", "Internal init command").Hidden()

		// start the container with planet
		cstart = app.Command("start", "Start Planet container")

		cstartMasterIP           = cstart.Flag("master-ip", "ip of the master pod").Default("127.0.0.1").OverrideDefaultFromEnvar("PLANET_MASTER_IP").IP()
		cstartCloudProvider      = cstart.Flag("cloud-provider", "cloud provider name, e.g. 'aws' or 'gce'").OverrideDefaultFromEnvar("PLANET_CLOUD_PROVIDER").String()
		cstartClusterID          = cstart.Flag("cluster-id", "id of the cluster").OverrideDefaultFromEnvar("PLANET_CLUSTER_ID").String()
		cstartIgnoreChecks       = cstart.Flag("ignore-checks", "Force start ignoring some failed host checks (e.g. kernel version)").OverrideDefaultFromEnvar("PLANET_FORCE").Bool()
		cstartEnv                = EnvVars(cstart.Flag("env", "Set environment variable").OverrideDefaultFromEnvar("PLANET_ENV"))
		cstartMounts             = Mounts(cstart.Flag("volume", "External volume to mount").OverrideDefaultFromEnvar("PLANET_VOLUME"))
		cstartRoles              = List(cstart.Flag("role", "Roles such as 'master' or 'node'").OverrideDefaultFromEnvar("PLANET_ROLE"))
		cstartInsecureRegistries = List(cstart.Flag("insecure-registry", "Optional insecure registries").OverrideDefaultFromEnvar("PLANET_INSECURE_REGISTRY"))

		// stop a running container
		cstop = app.Command("stop", "Stop planet container")

		// enter a running container
		center      = app.Command("enter", "Enter running planet container")
		centerArgs  = center.Arg("cmd", "command to execute").Default("/bin/bash").String()
		centerNoTTY = center.Flag("not-tty", "do not attach TTY to this process").Bool()
		centerUser  = center.Flag("user", "user to execute the command").Default("root").String()

		// report status of a running container
		cstatus = app.Command("status", "Get status of a running container")
	)

	cmd, err := app.Parse(os.Args[1:])
	if err != nil {
		return err
	}

	if *debug == true {
		log.Initialize("console", "INFO")
	} else {
		log.Initialize("console", "WARN")
	}

	var rootfs string
	switch cmd {

	// "start" command
	case cstart.FullCommand():
		rootfs, err = findRootfs()
		if err != nil {
			return err
		}
		setupSignalHanlders(rootfs)
		err = start(Config{
			Rootfs:             rootfs,
			Env:                *cstartEnv,
			Mounts:             *cstartMounts,
			IgnoreChecks:       *cstartIgnoreChecks,
			Roles:              *cstartRoles,
			InsecureRegistries: *cstartInsecureRegistries,
			MasterIP:           cstartMasterIP.String(),
			CloudProvider:      *cstartCloudProvider,
			ClusterID:          *cstartClusterID,
		})

	// "init" command
	case cinit.FullCommand():
		err = initLibcontainer()

	// "enter" command
	case center.FullCommand():
		rootfs, err = findRootfs()
		if err != nil {
			return err
		}
		err = enterConsole(
			rootfs, *centerArgs, *centerUser, !*centerNoTTY)

	// "stop" command
	case cstop.FullCommand():
		rootfs, err = findRootfs()
		if err != nil {
			return err
		}
		err = stop(rootfs)

	// "status" command
	case cstatus.FullCommand():
		rootfs, err = findRootfs()
		if err != nil {
			return err
		}
		err = status(rootfs)
	default:
		err = trace.Errorf("unsupported command: %v", cmd)
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

func enterConsole(rootfs, cmd, user string, tty bool) error {
	cfg := box.ProcessConfig{
		In:     os.Stdin,
		Out:    os.Stdout,
		Args:   []string{cmd},
		NoWait: false,
	}

	if tty {
		s, err := term.GetWinsize(os.Stdin.Fd())
		if err != nil {
			return trace.Wrap(err)
		}
		cfg.TTY = &box.TTY{H: int(s.Height), W: int(s.Width)}
	}

	return enter(rootfs, cfg)
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
	return trace.Errorf("this line should have never been executed")
}

func findRootfs() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", trace.Wrap(err, "failed to get current directory")
	}

	rootfs := filepath.Join(cwd, "rootfs")
	s, err := os.Stat(rootfs)
	if err != nil {
		return "", trace.Wrap(err, "rootfs error")
	}

	if !s.IsDir() {
		return "", trace.Errorf("rootfs is not a directory")
	}

	return rootfs, nil
}

// setupSignalHanlders sets up a handler to interrupt SIGTERM and SIGINT
// allowing for a graceful shutdown via executing "stop" command
func setupSignalHanlders(rootfs string) {
	c := make(chan os.Signal, 1)
	go func() {
		sig := <-c
		log.Infof("received a signal %v. stopping...\n", sig)
		err := stop(rootfs)
		if err != nil {
			log.Errorf("error: %v", err)
		}
	}()
	signal.Notify(c, os.Interrupt, os.Kill, syscall.SIGTERM)
}
