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
	"os"

	"github.com/gravitational/planet/lib/box"
	"github.com/gravitational/planet/lib/constants"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

type enterConfig struct {
	cmd     string
	user    string
	tty     bool
	stdin   bool
	args    []string
	seLinux bool
}

func enterConsole(config enterConfig) error {
	cfg := box.EnterConfig{
		Process: box.ProcessConfig{
			Out:  os.Stdout,
			Args: append([]string{config.cmd}, config.args...),
			Env: box.EnvVars{
				box.EnvPair{
					Name: EnvPath,
					Val:  DefaultEnvPath,
				},
				box.EnvPair{
					Name: EnvContainerRuntimeEndpoint,
					Val:  DefaultContainerRuntimeEndpoint,
				},
			},
		},
		SELinux: config.seLinux,
	}

	// tty allocation implies stdin
	if config.stdin || config.tty {
		cfg.Process.In = os.Stdin
	}

	if config.tty {
		s, err := box.GetWinsize(os.Stdin.Fd())
		if err != nil {
			return trace.Wrap(err, "error retrieving window size of tty")
		}
		cfg.Process.TTY = &box.TTY{H: int(s.Height), W: int(s.Width)}
	}

	return trace.Wrap(enter(cfg))
}

// enter initiates the process in the namespaces of the container
// managed by the planet master process and mantains websocket connection
// to proxy input and output
func enter(cfg box.EnterConfig) error {
	// tell bash to use environment we've created
	cfg.Process.Env.Upsert("ENV", containerEnvironmentFile)
	cfg.Process.Env.Upsert("BASH_ENV", containerEnvironmentFile)
	cfg.Process.Env.Upsert(EnvKubeConfig, constants.KubectlConfigPath)

	return trace.Wrap(box.Enter(cfg))
}

// stop interacts with systemctl's halt feature
func stop(seLinux bool) error {
	log.Info("Stop container.")
	cfg := box.EnterConfig{
		Process: box.ProcessConfig{
			User:         "root",
			Args:         []string{"/bin/systemctl", "halt"},
			In:           os.Stdin,
			Out:          os.Stdout,
			ProcessLabel: constants.ContainerInitProcessLabel,
		},
		SELinux: seLinux,
	}
	return trace.Wrap(enter(cfg))
}
