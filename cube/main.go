package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"

	"github.com/gravitational/trace"
	"github.com/opencontainers/runc/libcontainer"
)

func main() {
	fmt.Printf("called: %v\n", os.Args)

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
	default:
		err = trace.Errorf("unsupported command: %v", cmd)
	}

	if err != nil {
		log.Fatalf("cube error: %v", err)
	}

	log.Printf("cube: execution completed successfully")
}

func enterCmd() error {
	args := os.Args[2:]
	if len(args) < 1 {
		return trace.Errorf("cube enter path [command]")
	}
	path := args[0]
	command := "/bin/bash"
	if len(args) > 1 {
		command = args[1]
	}
	return enter(path, command)
}

func startCmd() error {
	var cfg CubeConfig
	cfg.Env = EnvVars{}

	args := os.Args[2:]
	fs := flag.FlagSet{}

	fs.StringVar(&cfg.MasterIP, "master-ip", "127.0.0.1", "master ip")
	fs.StringVar(&cfg.CloudProvider, "cloud-provider", "", "cloud provider")
	fs.StringVar(&cfg.CloudConfig, "cloud-config", "", "cloud config")
	fs.BoolVar(&cfg.Force, "force", true, "force start")
	fs.Var(&cfg.Env, "env", "set environment variable")
	if err := fs.Parse(args); err != nil {
		return trace.Wrap(err)
	}
	posArgs := fs.Args()
	log.Printf("args: %v", posArgs)
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
	log.Printf("initCmd")
	if err := factory.StartInitialization(); err != nil {
		log.Fatal(err)
	}
	return trace.Errorf("this line should have never been executed")
}
