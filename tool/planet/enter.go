package main

import (
	"bytes"
	"encoding/json"
	"os"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/docker/docker/pkg/term"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/planet/lib/box"
	"github.com/gravitational/planet/lib/monitoring"
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

// status checks status of the running planet cluster and outputs results as
// JSON to stdout.
func status(rootfs string) error {
	var (
		systemStatus *monitoring.SystemStatus
		data         []byte
		err          error
	)

	log.Infof("checking status in %s", rootfs)

	systemStatus, err = monitoring.Status()
	if err != nil {
		return trace.Wrap(err, "failed to check system status")
	}
	data, err = json.Marshal(systemStatus)
	if err != nil {
		return trace.Wrap(err, "failed to output status")
	}
	if _, err = os.Stdout.Write(data); err != nil {
		return trace.Wrap(err, "failed to output status")
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
