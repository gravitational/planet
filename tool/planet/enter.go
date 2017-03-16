package main

import (
	"bytes"
	"os"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/term"
	"github.com/gravitational/planet/lib/box"
	"github.com/gravitational/trace"
)

func enterConsole(rootfs, socketPath, cmd, user string, tty bool, args []string) (err error) {
	cfg := &box.ProcessConfig{
		In:   os.Stdin,
		Out:  os.Stdout,
		Args: append([]string{cmd}, args...),
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
	log.Infof("enter: %v %#v", rootfs, cfg)
	if cfg.TTY != nil {
		oldState, err := term.SetRawTerminal(os.Stdin.Fd())
		if err != nil {
			return err
		}
		defer term.RestoreTerminal(os.Stdin.Fd(), oldState)
	}
	// tell bash to use environment we've created
	cfg.Env.Upsert("ENV", ContainerEnvironmentFile)
	cfg.Env.Upsert("BASH_ENV", ContainerEnvironmentFile)
	cfg.Env.Upsert(EnvEtcdctlCertFile, DefaultEtcdctlCertFile)
	cfg.Env.Upsert(EnvEtcdctlKeyFile, DefaultEtcdctlKeyFile)
	cfg.Env.Upsert(EnvEtcdctlCAFile, DefaultEtcdctlCAFile)
	cfg.Env.Upsert(EnvEtcdctlPeers, DefaultEtcdEndpoints)
	cfg.Env.Upsert(EnvKubeConfig, KubeConfigPath)
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
		User: "root",
		Args: []string{"/bin/systemctl", "halt"},
		In:   os.Stdin,
		Out:  os.Stdout,
	}

	return enter(rootfs, socketPath, cfg)
}

// enterCommand is a helper function that runs a command as a root
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
