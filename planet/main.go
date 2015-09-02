package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/docker/docker/pkg/term"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/orbit/lib/box"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/opencontainers/runc/libcontainer"
	"github.com/gravitational/planet/Godeps/_workspace/src/gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	var (
		app   = kingpin.New("planet", "Planet is a Kubernetes delivered as an orbit container")
		debug = app.Flag("debug", "Enable debug mode").Bool()

		// internal init command used by libcontainer
		cinit = app.Command("init", "Internal init command").Hidden()

		// start the container with planet
		cstart = app.Command("start", "Start Planet container")

		cstartMasterIP      = cstart.Flag("master-ip", "ip of the master pod").Default("127.0.0.1").OverrideDefaultFromEnvar("PLANET_MASTER_IP").IP()
		cstartCloudProvider = cstart.Flag("cloud-provider", "cloud provider name, e.g. 'aws' or 'gce'").OverrideDefaultFromEnvar("PLANET_CLOUD_PROVIDER").String()
		cstartCloudConfig   = cstart.Flag("cloud-config", "cloud config path").OverrideDefaultFromEnvar("PLANET_CLOUD_CONFIG").String()
		cstartForce         = cstart.Flag("force", "Force start ignoring some failed host checks (e.g. kernel version)").OverrideDefaultFromEnvar("PLANET_FORCE").Bool()
		cstartEnv           = EnvVars(cstart.Flag("env", "Set environment variable").OverrideDefaultFromEnvar("PLANET_ENV"))
		cstartMounts        = Mounts(cstart.Flag("volume", "External volume to mount").OverrideDefaultFromEnvar("PLANET_VOLUME"))
		cstartRoles         = Roles(cstart.Flag("role", "Roles such as 'master' or 'node'").OverrideDefaultFromEnvar("PLANET_ROLE"))

		// stop a running container
		cstop = app.Command("stop", "Stop cube container")

		// enter a running container
		center      = app.Command("enter", "Enter running cube container")
		centerArgs  = center.Arg("cmd", "command to execute").Default("/bin/bash").String()
		centerNoTTY = center.Flag("not-tty", "do not attach TTY to this process").Bool()
		centerUser  = center.Flag("user", "user to execute the command").Default("root").String()

		// report status of a running container
		cstatus = app.Command("status", "Get status of a running container")
	)

	cmd, err := app.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "planet error: %v\n", err)
		os.Exit(-1)
	}

	rootfs, err := findRootfs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "planet error: %v\n", err)
		os.Exit(-1)
	}

	if *debug == true {
		log.Initialize("console", "INFO")
	} else {
		log.Initialize("console", "WARN")
	}

	switch cmd {
	case cstart.FullCommand():
		err = start(Config{
			Rootfs:        rootfs,
			Env:           *cstartEnv,
			Mounts:        *cstartMounts,
			Force:         *cstartForce,
			Roles:         *cstartRoles,
			MasterIP:      cstartMasterIP.String(),
			CloudProvider: *cstartCloudProvider,
			CloudConfig:   *cstartCloudConfig,
		})
	case cinit.FullCommand():
		err = initLibcontainer()
	case center.FullCommand():
		err = enterConsole(
			rootfs, *centerArgs, *centerUser, !*centerNoTTY)
	case cstop.FullCommand():
		err = stop(rootfs)
	case cstatus.FullCommand():
		err = status(rootfs)
	default:
		err = trace.Errorf("unsupported command: %v", cmd)
	}

	if err != nil {
		log.Errorf("planet error: %v", err)
		os.Exit(255)
	}

	log.Infof("cube: execution completed successfully")
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

func Roles(s kingpin.Settings) *roles {
	r := new(roles)
	s.SetValue(r)
	return r
}

func enterConsole(rootfs, cmd, user string, tty bool) error {
	cfg := box.ProcessConfig{
		In:   os.Stdin,
		Out:  os.Stdout,
		Args: []string{cmd},
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
