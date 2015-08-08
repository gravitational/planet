package main

import (
	"os"

	"github.com/gravitational/cube/Godeps/_workspace/src/github.com/docker/docker/pkg/term"
	"github.com/gravitational/cube/Godeps/_workspace/src/github.com/gravitational/orbit/box"
)

// enter initiates the process in the namespaces of the container
// managed by the cube master process and mantains websocket connection
// to proxy input and output
func enter(path string, cfg box.ProcessConfig) error {
	oldState, err := term.SetRawTerminal(os.Stdin.Fd())
	if err != nil {
		return err
	}
	defer term.RestoreTerminal(os.Stdin.Fd(), oldState)

	s, err := box.Connect(path)
	if err != nil {
		return err
	}

	return s.Enter(cfg)
}

// stop interacts with systemctl's halt feature
func stop(path string) error {
	cfg := box.ProcessConfig{
		User: "root",
		Args: []string{"/bin/systemctl", "halt"},
		In:   os.Stdin,
		Out:  os.Stdout,
	}

	return enter(path, cfg)
}
