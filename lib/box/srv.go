package box

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"

	"github.com/gravitational/planet/Godeps/_workspace/src/code.google.com/p/go-uuid/uuid"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/opencontainers/runc/libcontainer"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/opencontainers/runc/libcontainer/configs"
)

type ContainerServer interface {
	Enter(cfg ProcessConfig) error
}

type Box struct {
	Process     *libcontainer.Process
	Container   libcontainer.Container
	ContainerID string
	l           net.Listener
}

func (b *Box) Close() error {
	var err error
	if err = b.Container.Destroy(); err != nil {
		log.Errorf("error:%v", err)
	}
	if err = b.l.Close(); err != nil {
		log.Errorf("error:%v", err)
	}
	return err
}

func (b *Box) Wait() (*os.ProcessState, error) {
	defer b.Close()
	st, err := b.Process.Wait()
	if e, ok := err.(*exec.ExitError); ok {
		return e.ProcessState, nil
	}
	return st, err
}

func Start(cfg Config) (*Box, error) {
	log.Infof("starting with config: %v", cfg)
	if os.Geteuid() != 0 {
		return nil, trace.Errorf("should be run as root")
	}

	rootfs, err := checkPath(cfg.Rootfs, false)
	if err != nil {
		return nil, err
	}

	if err := mountCgroups("/"); err != nil {
		return nil, err
	}

	log.Infof("starting container process in '%v'", rootfs)

	if len(cfg.EnvFiles) != 0 {
		for _, ef := range cfg.EnvFiles {
			log.Infof("writing environment file: %v", ef.Env)
			err = writeEnvironment(filepath.Join(rootfs, ef.Path), ef.Env)
			if err != nil {
				return nil, err
			}
		}
	}

	root, err := libcontainer.New(cfg.DataDir, libcontainer.Cgroupfs)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	containerID := uuid.New()

	config := &configs.Config{
		Rootfs:       rootfs,
		Capabilities: cfg.Capabilities,
		Namespaces: configs.Namespaces([]configs.Namespace{
			{Type: configs.NEWNS},
			{Type: configs.NEWUTS},
			{Type: configs.NEWIPC},
			{Type: configs.NEWPID},
		}),
		Mounts: []*configs.Mount{
			{
				Source:      "/proc",
				Destination: "/proc",
				Device:      "proc",
				Flags:       defaultMountFlags,
			},
			// this is needed for flanneld that does modprobe
			{
				Device:      "bind",
				Source:      "/lib/modules",
				Destination: "/lib/modules",
				Flags:       defaultMountFlags | syscall.MS_BIND,
			},
			// don't mount real dev, otherwise systemd will mess up with the host
			// OS real badly
			{
				Source:      "tmpfs",
				Destination: "/dev",
				Device:      "tmpfs",
				Flags:       syscall.MS_NOSUID | syscall.MS_STRICTATIME,
				Data:        "mode=755",
			},
			{
				Source:      "sysfs",
				Destination: "/sys",
				Device:      "sysfs",
				Flags:       defaultMountFlags | syscall.MS_RDONLY,
			},
			{
				Source:      "devpts",
				Destination: "/dev/pts",
				Device:      "devpts",
				Flags:       syscall.MS_NOSUID | syscall.MS_NOEXEC,
				Data:        "newinstance,ptmxmode=0666,mode=0620,gid=5",
			},
		},
		Cgroups: &configs.Cgroup{
			Name:            containerID,
			Parent:          "system",
			AllowAllDevices: false,
			AllowedDevices:  configs.DefaultAllowedDevices,
		},

		Devices:  configs.DefaultAutoCreatedDevices,
		Hostname: containerID,
	}

	for _, m := range cfg.Mounts {
		src, err := checkPath(m.Src, false)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		config.Mounts = append(config.Mounts, &configs.Mount{
			Device:      "bind",
			Source:      src,
			Destination: m.Dst,
			Flags:       syscall.MS_BIND,
		})
	}

	container, err := root.Create(containerID, config)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	st, err := container.Status()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	log.Infof("container status: %v %v", st, err)

	process := &libcontainer.Process{
		Args:   cfg.InitArgs,
		Env:    cfg.InitEnv,
		User:   cfg.InitUser,
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}

	// this will cause libcontainer to exec this binary again
	// with "init" command line argument.  (this is the default setting)
	// then our init() function comes into play
	if err := container.Start(process); err != nil {
		return nil, trace.Wrap(err)
	}

	l, err := startWebServer(serverSockPath(cfg.Rootfs), container)
	if err != nil {
		return nil, err
	}

	return &Box{
		Process:     process,
		ContainerID: containerID,
		Container:   container,
		l:           l}, nil
}

func getEnvironment(env EnvVars) []string {
	out := make([]string, len(env))
	for i, v := range env {
		out[i] = fmt.Sprintf("%v=%v\n", v.Name, v.Val)
	}
	return out
}

func writeEnvironment(path string, env EnvVars) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return trace.Wrap(err)
	}
	f, err := os.Create(path)
	if err != nil {
		return trace.Wrap(err)
	}
	defer f.Close()
	for _, v := range env {
		if _, err := fmt.Fprintf(f, "%v=%v\n", v.Name, v.Val); err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

func checkPath(p string, executable bool) (string, error) {
	if p == "" {
		return "", trace.Errorf("path to root filesystem can not be empty")
	}
	cp, err := filepath.Abs(p)
	if err != nil {
		return "", trace.Wrap(err)
	}
	fi, err := os.Stat(cp)
	if err != nil {
		return "", trace.Wrap(err)
	}
	if executable && (fi.Mode()&0111 == 0) {
		return "", trace.Errorf("file %v is not executable", cp)
	}
	return cp, nil
}

const defaultMountFlags = syscall.MS_NOEXEC | syscall.MS_NOSUID | syscall.MS_NODEV

func writeConfig(target, source string) error {
	bytes := []byte{}
	var err error
	if source != "" {
		bytes, err = ioutil.ReadFile(source)
		if err != nil {
			return err
		}
	}
	if err != nil {
		return trace.Wrap(ioutil.WriteFile(target, bytes, 0644))
	}
	return nil
}

func startWebServer(path string, c libcontainer.Container) (net.Listener, error) {
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	srv := &http.Server{
		Handler: NewWebServer(c),
	}
	go func() {
		defer func() {
			if err := l.Close(); err != nil {
				log.Warningf("failed to remove socket file: %v", err)
			}
		}()
		if err := srv.Serve(l); err != nil {
			log.Infof("server stopped with: %v", err)
		}
	}()
	return l, nil
}

func serverSockPath(p string) string {
	return filepath.Join(p, "run", "cube.socket")
}
