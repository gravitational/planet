package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/docker/docker/pkg/term"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/planet/lib/box"
)

// enter initiates the process in the namespaces of the container
// managed by the planet master process and mantains websocket connection
// to proxy input and output
func enter(rootfs string, cfg box.ProcessConfig) error {
	log.Infof("enter: %v %#v", rootfs, cfg)
	if cfg.TTY != nil {
		oldState, err := term.SetRawTerminal(os.Stdin.Fd())
		if err != nil {
			return err
		}
		defer term.RestoreTerminal(os.Stdin.Fd(), oldState)
	}
	s, err := box.Connect(rootfs)
	if err != nil {
		return err
	}

	return s.Enter(cfg)
}

// stop interacts with systemctl's halt feature
func stop(path string) error {
	log.Infof("stop: %v", path)
	cfg := box.ProcessConfig{
		User: "root",
		Args: []string{"/bin/systemctl", "halt"},
		In:   os.Stdin,
		Out:  os.Stdout,
	}

	return enter(path, cfg)
}

// status checks status of all units to see if there are any failed units
// it outputs status in the structured json format
func status(rootfs string) error {
	log.Infof("status: %v", rootfs)
	out, err := enterCommand(rootfs, []string{"/bin/systemctl", "--failed"})
	if err != nil {
		if !box.IsConnectError(err) {
			return trace.Wrap(err)
		}
		return printStatus(Status{Status: StatusStopped})
	}
	if !strings.Contains(out, "0 loaded units listed") {
		return printStatus(Status{Status: StatusDegraded, Info: out})
	}
	// some units may be still loading, report that
	out, err = enterCommand(rootfs, []string{"/bin/systemctl", "--state=load"})
	if err != nil {
		if !box.IsConnectError(err) {
			return trace.Wrap(err)
		}
		return printStatus(Status{Status: StatusStopped})
	}
	if !strings.Contains(out, "0 loaded units listed") {
		return printStatus(Status{Status: StatusLoading, Info: out})
	}
	return printStatus(Status{Status: StatusRunning, Info: out})
}

func printStatus(s Status) error {
	bytes, err := json.Marshal(s)
	if err != nil {
		return trace.Wrap(err)
	}
	_, err = os.Stdout.Write(bytes)
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// enterCommand is a helper function that runs a command as a root
// in the namespace of planet's container. It returns error
// if command failed, or command standard output otherwise
func enterCommand(rootfs string, args []string) (string, error) {
	out := &bytes.Buffer{}
	cfg := box.ProcessConfig{
		User: "root",
		Args: args,
		In:   os.Stdin,
		Out:  out,
	}
	err := enter(rootfs, cfg)
	if err != nil {
		return "", trace.Wrap(err)
	}
	return out.String(), nil
}

const (
	// StatusRunning means that all units inside planet are up and running
	StatusRunning = "running"
	// StatusDegraded means that some units have failed
	StatusDegraded = "degraded"
	// StatusStopped means that planet is stopped
	StatusStopped = "stopped"
	// StatusLoading means that some units are still loading
	StatusLoading = "loading"
)

// Status is as tructured status returned by the planet status command
type Status struct {
	// Status of the running container
	// one of 'running', 'stopped', 'degraded' or 'loading'
	Status string `json:"status"`
	// App-specific information about the container status
	Info interface{} `json:"info"`
}
