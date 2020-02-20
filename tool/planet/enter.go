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
	"path/filepath"

	"github.com/docker/docker/pkg/term"
	"github.com/gravitational/planet/lib/box"
	"github.com/gravitational/planet/lib/constants"

	//"github.com/gravitational/planet/lib/term"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

func enterConsole(rootfs, socketPath, cmd, user string, tty bool, stdin bool, args []string) (err error) {
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
		s, err := term.GetWinsize(os.Stdin.Fd())
		if err != nil {
			return trace.Wrap(err, "error retrieving windows size of tty")
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
	//log.Infof("enter: %v %#v", rootfs, cfg)

	//fmt.Fprintln(os.Stdout, "Running enter")
	/*
		if cfg.TTY != nil {
			fmt.Fprintln(os.Stdout, "Setting Raw Terminal")
			oldState, err := term.SetRawTerminal(os.Stdin.Fd())
			if err != nil {
				return err
			}

			defer func() {
				fmt.Fprintln(os.Stdout, "Restoring old terminal")
				term.RestoreTerminal(os.Stdin.Fd(), oldState)
			}()
		}
	*/

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
	/*s, err := box.Connect(&box.ClientConfig{
		Rootfs:     rootfs,
		SocketPath: socketPath,
	})
	if err != nil {
		return err
	}

	return s.Enter(*cfg)
	*/
	//fmt.Println("using local terminal")
	exitCode, err := box.LocalEnter("/var/run/planet", cfg)
	if err != nil {
		return trace.Wrap(err)
	}
	os.Exit(exitCode)
	return nil
}

// stop interacts with systemctl's halt feature
func stop(rootfs, socketPath string) error {
	log.Infof("stop: %v", rootfs)
	cfg := &box.ProcessConfig{
		User: "root",
		Args: []string{"/bin/systemctl", "halt"},
		In:   os.Stdin,
		Out:  os.Stdout,
	}

	return enter(rootfs, socketPath, cfg)
}

// enterCommand is a helper function that runs a command as root
// in the namespace of planet's container. It returns error
// if command failed, or command standard output otherwise
func enterCommand(rootfs, socketPath string, args []string) ([]byte, error) {
	buf := &bytes.Buffer{}
	cfg := &box.ProcessConfig{
		User: "root",
		Args: args,
		In:   os.Stdin,
		Out:  buf,
	}
	err := enter(rootfs, socketPath, cfg)
	if err != nil {
		err = trace.Wrap(err)
	}
	return buf.Bytes(), err
}
