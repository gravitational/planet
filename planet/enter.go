package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/docker/docker/pkg/term"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/orbit/box"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
)

// enter initiates the process in the namespaces of the container
// managed by the cube master process and mantains websocket connection
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
func status(rootfs string) error {
	out := &bytes.Buffer{}
	log.Infof("status: %v", rootfs)
	cfg := box.ProcessConfig{
		User: "root",
		Args: []string{"/bin/systemctl", "--failed"},
		In:   os.Stdin,
		Out:  out,
	}

	var s box.Status
	err := enter(rootfs, cfg)
	if err != nil {
		if box.IsConnectError(err) {
			s.Status = box.StatusStopped
		} else {
			return err
		}
	} else {
		d := out.String()
		if !strings.Contains(d, "0 loaded units listed") {
			s.Status = box.StatusDegraded
			s.Info = d
		} else {
			s.Status = box.StatusRunning
			s.Info = d
		}
	}

	bytes, err := json.Marshal(s)
	if err != nil {
		return trace.Wrap(err, "failed to serialize status")
	}

	if _, err := os.Stdout.Write(bytes); err != nil {
		return trace.Wrap(err, "failed to output status")
	}

	return nil
}
