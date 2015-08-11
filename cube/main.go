package main

import (
	"flag"
	"os"
	"runtime"

	"github.com/gravitational/cube/Godeps/_workspace/src/github.com/gravitational/orbit/box"

	"github.com/gravitational/cube/Godeps/_workspace/src/github.com/docker/docker/pkg/term"
	"github.com/gravitational/cube/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/cube/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/cube/Godeps/_workspace/src/github.com/opencontainers/runc/libcontainer"
)

func main() {
	log.Initialize("console", "WARN")

	if len(os.Args) < 2 {
		log.Fatalf("specify a command, one of 'start', 'enter', 'stop'")
	}

	cmd := os.Args[1]

	var err error
	switch cmd {
	case "start":
		err = startCmd()
	case "init":
		err = initCmd()
	case "enter":
		err = enterCmd()
	case "stop":
		err = stopCmd()
	default:
		err = trace.Errorf("unsupported command: %v", cmd)
	}

	if err != nil {
		log.Errorf("cube error: %v", err)
		os.Exit(255)
	}

	log.Infof("cube: execution completed successfully")
}

func stopCmd() error {
	args := os.Args[2:]
	fs := flag.FlagSet{} // want to add wait flag later
	if err := fs.Parse(args); err != nil {
		return trace.Wrap(err)
	}

	posArgs := fs.Args()
	log.Infof("args: %v", posArgs)

	if len(posArgs) < 1 {
		return trace.Errorf("cube enter path [command]")
	}
	return stop(posArgs[0])
}

func enterCmd() error {
	args := os.Args[2:]
	fs := flag.FlagSet{}

	cfg := box.ProcessConfig{
		In:  os.Stdin,
		Out: os.Stdout,
	}
	var tty bool

	fs.BoolVar(&tty, "tty", true, "attach terminal (for interactive sessions)")
	fs.StringVar(&cfg.User, "user", "root", "user running the process")
	if err := fs.Parse(args); err != nil {
		return trace.Wrap(err)
	}

	posArgs := fs.Args()
	log.Infof("args: %v", posArgs)

	if len(posArgs) < 1 {
		return trace.Errorf("cube enter path [command]")
	}
	path := posArgs[0]
	cfg.Args = []string{"/bin/bash"}
	if len(args) > 1 {
		cfg.Args = posArgs[1:]
	}

	if tty {
		s, err := term.GetWinsize(os.Stdin.Fd())
		if err != nil {
			return trace.Wrap(err)
		}
		cfg.TTY = &box.TTY{H: int(s.Height), W: int(s.Width)}
	}

	return enter(path, cfg)
}

func startCmd() error {
	var cfg CubeConfig
	cfg.Env = box.EnvVars{}
	cfg.Mounts = box.Mounts{}

	args := os.Args[2:]
	fs := flag.FlagSet{}

	fs.StringVar(&cfg.MasterIP, "master-ip", "127.0.0.1", "master ip")
	fs.StringVar(&cfg.CloudProvider, "cloud-provider", "", "cloud provider")
	fs.StringVar(&cfg.CloudConfig, "cloud-config", "", "cloud config")
	fs.BoolVar(&cfg.Force, "force", true, "force start")
	fs.Var(&cfg.Env, "env", "set environment variable")
	fs.Var(&cfg.Mounts, "volume", "volume in form src:dst")
	fs.Var(&cfg.Roles, "role", "list of roles assigned to the cube instance")
	if err := fs.Parse(args); err != nil {
		return trace.Wrap(err)
	}

	posArgs := fs.Args()
	log.Infof("args: %v", posArgs)
	if len(posArgs) < 1 {
		return trace.Errorf("need path to root filesystem")
	}

	cfg.Rootfs = posArgs[0]

	return start(cfg)
}

// initCmd is implicitly called by the libcontainer logic and is used to start
// a process in the new namespaces and cgroups
func initCmd() error {
	runtime.GOMAXPROCS(1)
	runtime.LockOSThread()
	factory, _ := libcontainer.New("")
	if err := factory.StartInitialization(); err != nil {
		log.Fatalf("error: %v", err)
	}
	return trace.Errorf("this line should have never been executed")
}
