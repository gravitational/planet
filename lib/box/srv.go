package box

import (
	"fmt"
	"io"
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
	listener    net.Listener
}

// Close shuts down the box. It is written to be safe to call multiple
// times in a row for extra robustness.
func (b *Box) Close() error {
	var err error
	if b.Container != nil {
		if err = b.Container.Destroy(); err != nil {
			log.Errorf("box.Close() :%v", err)
		}
	}
	if b.listener != nil {
		if err = b.listener.Close(); err != nil {
			log.Warningf("box.Close(): %v", err)
		}
	}
	return err
}

func (b *Box) Wait() (*os.ProcessState, error) {
	log.Infof("box.Wait() is called")
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
			if err := writeEnvironment(filepath.Join(rootfs, ef.Path), ef.Env); err != nil {
				return nil, err
			}
		}
	}

	if len(cfg.Files) != 0 {
		for _, f := range cfg.Files {
			log.Errorf("writing file to: %v", filepath.Join(rootfs, f.Path))
			if err := writeFile(filepath.Join(rootfs, f.Path), f); err != nil {
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
		mnt := &configs.Mount{
			Device:      "bind",
			Source:      src,
			Destination: m.Dst,
			Flags:       syscall.MS_BIND,
		}
		if m.Readonly {
			mnt.Flags |= syscall.MS_RDONLY
		}
		config.Mounts = append(config.Mounts, mnt)
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

	// start the API webserver (the sooner the better, so if it can't start we can
	// fail sooner)
	listener, err := startWebServer(serverSockPath(cfg.Rootfs), container)
	if err != nil {
		return nil, err
	}

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
		// kill the webserver (so it would close the socket)
		listener.Close()
		return nil, trace.Wrap(err)
	}

	return &Box{
		Process:     process,
		ContainerID: containerID,
		Container:   container,
		listener:    listener}, nil
}

func getEnvironment(env EnvVars) []string {
	out := make([]string, len(env))
	for i, v := range env {
		out[i] = fmt.Sprintf("%v=%v\n", v.Name, v.Val)
	}
	return out
}

func writeFile(path string, fi File) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return trace.Wrap(err)
	}
	f, err := os.Create(path)
	if err != nil {
		return trace.Wrap(err)
	}
	defer f.Close()
	if _, err := io.Copy(f, fi.Contents); err != nil {
		return trace.Wrap(err)
	}
	if fi.Mode != 0 {
		if err := f.Chmod(fi.Mode); err != nil {
			return trace.Wrap(err)
		}
	}
	if fi.Owners != nil {
		if err := f.Chown(fi.Owners.UID, fi.Owners.GID); err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
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

// startWebServer creates a listening socket on a given path (like /var/run/planet.sock)
// this function leaves a running goroutine behind
func startWebServer(socketPath string, c libcontainer.Container) (net.Listener, error) {
	l, err := net.Listen("unix", socketPath)
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
	return filepath.Join(p, "run", "planet.socket")
}
