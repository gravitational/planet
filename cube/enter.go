package main

import (
	"bytes"
	"os"
	"strings"

	"github.com/gravitational/cube/Godeps/_workspace/src/github.com/docker/docker/pkg/term"
	"github.com/gravitational/cube/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/cube/Godeps/_workspace/src/github.com/gravitational/orbit/box"
	"github.com/gravitational/cube/Godeps/_workspace/src/github.com/gravitational/trace"
)

// enter initiates the process in the namespaces of the container
// managed by the cube master process and mantains websocket connection
// to proxy input and output
func enter(rootfs string, cfg box.ProcessConfig) error {
	log.Infof("enter: %v %#v", rootfs, cfg)

	oldState, err := term.SetRawTerminal(os.Stdin.Fd())
	if err != nil {
		return err
	}
	defer term.RestoreTerminal(os.Stdin.Fd(), oldState)

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

	if err := enter(rootfs, cfg); err != nil {
		return err
	}

	d := out.String()
	if !strings.Contains(d, "0 loaded units listed") {
		return trace.Errorf("error: %v", d)
	}

	log.Infof("all units are operational:\n %v", d)
	return nil
}
