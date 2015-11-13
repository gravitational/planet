package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/docker/docker/pkg/term"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/orbit/lib/pkg"
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

// status checks status of the running planet cluster and outputs results as
// JSON to stdout.
func status(rootfs string) error {
	var (
		err    error
		result bytes.Buffer
	)

	log.Infof("checking status in %s", rootfs)
	err = checkSystemdStatus(rootfs, &result)
	if err != nil {
		return trace.Wrap(err, "failed to check systemd status")
	}

	err = checkAlerts(rootfs, &result)
	if err != nil {
		return trace.Wrap(err, "failed to check alerts")
	}

	if _, err := os.Stdout.Write(result.Bytes()); err != nil {
		return trace.Wrap(err, "failed to output status")
	}

	return nil
}

func checkSystemdStatus(rootfs string, result io.Writer) error {
	var (
		err    error
		output bytes.Buffer
		cfg    = box.ProcessConfig{
			User: "root",
			Args: []string{"/bin/systemctl", "--failed"},
			In:   os.Stdin,
			Out:  &output,
		}
		s      pkg.Status
		stdout string
	)

	err = enter(rootfs, cfg)
	if err != nil {
		if box.IsConnectError(err) {
			s.Status = pkg.StatusStopped
		} else {
			return trace.Wrap(err)
		}
	} else {
		stdout = output.String()
		s.Info = stdout
		// FIXME: avoid false positives with failures >= 10
		if !strings.Contains(stdout, "0 loaded units listed") {
			s.Status = pkg.StatusDegraded
		} else {
			s.Status = pkg.StatusRunning
		}
	}

	enc := json.NewEncoder(result)
	if err = enc.Encode(s); err != nil {
		return trace.Wrap(err, "failed to serialize status")
	}

	return nil
}

func checkAlerts(rootfs string, result io.Writer) error {
	var (
		err    error
		output bytes.Buffer
		cfg    = box.ProcessConfig{
			User: "root",
			Args: []string{"cat", alertsFile},
			In:   os.Stdin,
			Out:  &output,
		}
	)

	err = enter(rootfs, cfg)
	if err != nil && !box.IsConnectError(err) {
		return trace.Wrap(err)
	}
	return nil
}
