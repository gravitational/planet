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

	"github.com/gravitational/planet/lib/box"
	"github.com/gravitational/planet/lib/constants"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

func enterConsole(cmd, user string, tty bool, stdin bool, args []string) error {
	cfg := &box.ProcessConfig{
		Out:  os.Stdout,
		Args: append([]string{cmd}, args...),
		Env: box.EnvVars{
			box.EnvPair{
				Name: EnvPath,
				Val:  DefaultEnvPath,
			},
		},
	}

	// tty allocation implies stdin
	if stdin || tty {
		cfg.In = os.Stdin
	}

	if tty {
		s, err := box.GetWinsize(os.Stdin.Fd())
		if err != nil {
			return trace.Wrap(err, "error retrieving windows size of tty")
		}
		cfg.TTY = &box.TTY{H: int(s.Height), W: int(s.Width)}
	}

	return trace.Wrap(enter(cfg))
}

// enter initiates the process in the namespaces of the container
// managed by the planet master process and mantains websocket connection
// to proxy input and output
func enter(cfg *box.ProcessConfig) error {
	// tell bash to use environment we've created
	cfg.Env.Upsert("ENV", ContainerEnvironmentFile)
	cfg.Env.Upsert("BASH_ENV", ContainerEnvironmentFile)
	cfg.Env.Upsert(EnvKubeConfig, constants.KubectlConfigPath)

	return trace.Wrap(box.Enter("/var/run/planet", cfg))
}

// stop interacts with systemctl's halt feature
func stop() error {
	log.Infof("stop planet container")
	cfg := &box.ProcessConfig{
		User: "root",
		Args: []string{"/bin/systemctl", "halt"},
		In:   os.Stdin,
		Out:  os.Stdout,
	}

	return trace.Wrap(enter(cfg))
}

// enterCommand is a helper function that runs a command as root
// in the namespace of planet's container. It returns error
// if command failed, or command standard output otherwise
func enterCommand(args []string) ([]byte, error) {
	buf := &bytes.Buffer{}
	cfg := &box.ProcessConfig{
		User: "root",
		Args: args,
		In:   os.Stdin,
		Out:  buf,
	}
	return buf.Bytes(), trace.Wrap(enter(cfg))
}
