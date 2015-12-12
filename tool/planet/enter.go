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

func enterConsole(config *box.ClientConfig, cmd, user string, tty bool, args []string) (err error) {
	process := &box.ProcessConfig{
		In:   os.Stdin,
		Out:  os.Stdout,
		Args: append([]string{cmd}, args...),
	}

	if tty {
		s, err := term.GetWinsize(os.Stdin.Fd())
		if err != nil {
			return trace.Wrap(err)
		}
		process.TTY = &box.TTY{H: int(s.Height), W: int(s.Width)}
	}

	err = enter(config, process)
	return err
}

// enter initiates the process in the namespaces of the container
// managed by the planet master process and mantains websocket connection
// to proxy input and output
func enter(config *box.ClientConfig, process *box.ProcessConfig) error {
	log.Infof("enter: %v %#v", config.Rootfs, process)
	if process.TTY != nil {
		oldState, err := term.SetRawTerminal(os.Stdin.Fd())
		if err != nil {
			return err
		}
		defer term.RestoreTerminal(os.Stdin.Fd(), oldState)
	}
	// tell bash to use environment we've created
	process.Env.Upsert("ENV", ContainerEnvironmentFile)
	process.Env.Upsert("BASH_ENV", ContainerEnvironmentFile)
	s, err := box.Connect(config)
	if err != nil {
		return err
	}

	return s.Enter(*process)
}

// stop interacts with systemctl's halt feature
func stop(config *box.ClientConfig) error {
	log.Infof("stop: %v", config.Rootfs)
	process := &box.ProcessConfig{
		User: "root",
		Args: []string{"/bin/systemctl", "halt"},
		In:   os.Stdin,
		Out:  os.Stdout,
	}

	return enter(config, process)
}

// status checks status of the running planet cluster and outputs results as
// JSON to stdout.
func status(config *box.ClientConfig) (err error) {
	log.Infof("checking status in %s", config.Rootfs)

	var statusCmd = []string{"/usr/bin/planet", "--from-container", "status"}
	var data []byte

	data, err = enterCommand(config, statusCmd)
	if data != nil {
		if _, errWrite := os.Stdout.Write(data); errWrite != nil {
			return trace.Wrap(errWrite, "failed to output status")
		}
	}
	return err
}

// containerStatus reports the current cluster status.
// It assumes the context of the planet container.
func containerStatus() (ok bool, err error) {
	systemStatus, err := monitoring.Status()
	if err != nil {
		return false, trace.Wrap(err, "failed to check system status")
	}
	ok = systemStatus.Status == monitoring.SystemStatusRunning

	data, err := json.Marshal(systemStatus)
	if err != nil {
		return ok, trace.Wrap(err, "failed to unmarshal status data")
	}
	if _, err = os.Stdout.Write(data); err != nil {
		return ok, trace.Wrap(err, "failed to output status")
	}

	return ok, nil
}

// enterCommand is a helper function that runs a command as a root
// in the namespace of planet's container. It returns error
// if command failed, or command standard output otherwise
func enterCommand(config *box.ClientConfig, args []string) ([]byte, error) {
	buf := &bytes.Buffer{}
	process := &box.ProcessConfig{
		User: "root",
		Args: args,
		In:   os.Stdin,
		Out:  buf,
	}
	err := enter(config, process)
	if err != nil {
		err = trace.Wrap(err)
	}
	return buf.Bytes(), err
}
