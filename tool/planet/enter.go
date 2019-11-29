/*
Copyright 2018 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gravitational/planet/lib/box"
	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/defaults"

	"github.com/docker/docker/pkg/term"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

func enterConsole(rootfs, socketPath, cmd, user string, tty bool, stdin bool, args []string) (err error) {
	label, err := getProcLabel(filepath.Join(rootfs, cmd))
	if err != nil {
		log.WithFields(log.Fields{
			log.ErrorKey: err,
			"path":       cmd,
		}).Warn("Failed to compute process label.")
		label = defaults.ContainerProcessLabel
	}
	cfg := &box.ProcessConfig{
		Out:  os.Stdout,
		Args: append([]string{cmd}, args...),
		Env: box.EnvVars{
			box.EnvPair{
				Name: EnvPath,
				Val:  DefaultEnvPath,
			},
		},
		ProcessLabel: label,
	}

	// tty allocation implies stdin
	if stdin || tty {
		cfg.In = os.Stdin
	}

	if tty {
		s, err := term.GetWinsize(os.Stdin.Fd())
		if err != nil {
			return trace.Wrap(err)
		}
		cfg.TTY = &box.TTY{H: int(s.Height), W: int(s.Width)}
	}

	err = enter(rootfs, socketPath, cfg)
	return err
}

// enter initiates the process in the namespaces of the container
// managed by the planet master process and mantains websocket connection
// to proxy input and output
func enter(rootfs, socketPath string, cfg *box.ProcessConfig) error {
	log.WithFields(log.Fields{
		"rootfs": rootfs,
		"config": cfg,
	}).Debug("Enter.")
	if cfg.TTY != nil {
		oldState, err := term.SetRawTerminal(os.Stdin.Fd())
		if err != nil {
			return err
		}
		defer term.RestoreTerminal(os.Stdin.Fd(), oldState)
	}

	env, err := box.ReadEnvironment(filepath.Join(rootfs, ProxyEnvironmentFile))
	if err != nil {
		return trace.Wrap(err)
	}
	for _, e := range env {
		cfg.Env.Upsert(e.Name, e.Val)
	}

	// tell bash to use environment we've created
	cfg.Env.Upsert("ENV", ContainerEnvironmentFile)
	cfg.Env.Upsert("BASH_ENV", ContainerEnvironmentFile)
	cfg.Env.Upsert(EnvEtcdctlCertFile, DefaultEtcdctlCertFile)
	cfg.Env.Upsert(EnvEtcdctlKeyFile, DefaultEtcdctlKeyFile)
	cfg.Env.Upsert(EnvEtcdctlCAFile, DefaultEtcdctlCAFile)
	cfg.Env.Upsert(EnvEtcdctlPeers, DefaultEtcdEndpoints)
	cfg.Env.Upsert(EnvKubeConfig, constants.KubectlConfigPath)
	s, err := box.Connect(&box.ClientConfig{
		Rootfs:     rootfs,
		SocketPath: socketPath,
	})
	if err != nil {
		return err
	}

	return s.Enter(*cfg)
}

// stop interacts with systemctl's halt feature
func stop(rootfs, socketPath string) error {
	log.Infof("stop: %v", rootfs)
	cfg := &box.ProcessConfig{
		User:         "root",
		Args:         []string{"/bin/systemctl", "halt"},
		In:           os.Stdin,
		Out:          os.Stdout,
		ProcessLabel: constants.ContainerInitProcessLabel,
	}

	return enter(rootfs, socketPath, cfg)
}

// getProcLabel computes the label for the new process initiating from the file
// given wih path. The label is computed in the context of the init process.
func getProcLabel(path string) (label string, err error) {
	out, err := exec.Command("selinuxexeccon", path, constants.ContainerInitProcessLabel).CombinedOutput()
	if err != nil {
		return "", trace.Wrap(err, "failed to compute process label for %v: %s",
			path, out)
	}
	return string(bytes.TrimSpace(out)), nil
}
