package box

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/gravitational/go-udev"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"

	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/configs"
	"github.com/pborman/uuid"
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
	log.Infof("[BOX] starting with config: %#v", cfg)

	rootfs, err := checkPath(cfg.Rootfs, false)
	if err != nil {
		return nil, err
	}

	log.Infof("starting container process in '%v'", rootfs)

	if len(cfg.EnvFiles) != 0 {
		for _, ef := range cfg.EnvFiles {
			log.Infof("writing environment file: %v", ef.Env)
			if err := WriteEnvironment(filepath.Join(rootfs, ef.Path), ef.Env); err != nil {
				return nil, err
			}
		}
	}

	if len(cfg.Files) != 0 {
		for _, f := range cfg.Files {
			log.Infof("writing file to: %v", filepath.Join(rootfs, f.Path))
			if err := writeFile(filepath.Join(rootfs, f.Path), f); err != nil {
				return nil, err
			}
		}
	}

	root, err := libcontainer.New(cfg.DataDir, libcontainer.Cgroupfs)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// find all existing loop devices on a host and re-create them inside the container
	hostLoops, err := filepath.Glob("/dev/loop?")
	if err != nil {
		return nil, trace.Wrap(err)
	}
	loopDevices := make([]*configs.Device, len(hostLoops))
	for i := range hostLoops {
		loopDevices[i] = &configs.Device{
			Type:        'b',
			Path:        fmt.Sprintf("/dev/loop%d", i),
			Major:       7,
			Minor:       int64(i),
			Uid:         0,
			Gid:         0,
			Permissions: "rwm",
			FileMode:    0660,
		}
	}

	containerID := uuid.New()
	hostname, err := os.Hostname()
	if err != nil {
		return nil, trace.Wrap(err, "failed to get hostname")
	}

	// Enumerate all known block devices of type disk/partition
	udev := udev.Udev{}
	enum := udev.NewEnumerate()

	devices, err := enum.Devices()
	if err != nil {
		return nil, trace.Wrap(err, "failed to enumerate available devices")
	}

	var disks []*configs.Device
	for _, device := range devices {
		deviceType := device.Devtype()
		if deviceType == "disk" || deviceType == "partition" {
			devnum := device.Devnum()
			disks = append(disks, &configs.Device{
				Type:        'b',
				Path:        device.Devnode(),
				Major:       int64(devnum.Major()),
				Minor:       int64(devnum.Minor()),
				Permissions: "rwm",
				FileMode:    0660,
			})
		}
	}

	capabilities := configs.Capabilities{
		Bounding:    cfg.Capabilities,
		Effective:   cfg.Capabilities,
		Inheritable: cfg.Capabilities,
		Permitted:   cfg.Capabilities,
		Ambient:     cfg.Capabilities,
	}

	allowAllDevices := true
	config := &configs.Config{
		Rootfs:       rootfs,
		Capabilities: &capabilities,
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
			// needed for dynamically provisioned/attached persistent volumes
			// to work on some cloud providers (e.g. GCE) which use symlinks
			// in /dev/disk/by-uuid to refer to provisioned devices
			{
				Device:      "bind",
				Source:      "/dev/disk",
				Destination: "/dev/disk",
				Flags:       syscall.MS_BIND,
			},
		},
		Cgroups: &configs.Cgroup{
			Name:   containerID,
			Parent: "system",
			Resources: &configs.Resources{
				AllowAllDevices:  &allowAllDevices,
				AllowedDevices:   configs.DefaultAllowedDevices,
				MemorySwappiness: nil, // nil means "machine-default" and that's what we need because we don't care
			},
		},

		Devices:  append(configs.DefaultAutoCreatedDevices, append(loopDevices, disks...)...),
		Hostname: hostname,
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

	// start the API webserver (the sooner the better, so if it can't start we can
	// fail sooner)
	socketPath := serverSockPath(cfg.Rootfs, cfg.SocketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	err = startWebServer(listener, container)
	if err != nil {
		return nil, trace.Wrap(err)
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

	status, err := container.Status()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	log.Infof("container status: %v (err=%v)", status, err)

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

// WriteEnvironment writes provided environment variables to a file at the
// specified path.
func WriteEnvironment(path string, env EnvVars) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return trace.Wrap(err)
	}
	f, err := os.Create(path)
	if err != nil {
		return trace.Wrap(err)
	}
	defer f.Close()
	for _, v := range env {
		// quote value as it may contain spaces
		if _, err := fmt.Fprintf(f, "%v=%q\n", v.Name, v.Val); err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

// ReadEnvironment returns a list of all environment variables read from the file
// at the specified path.
func ReadEnvironment(path string) (vars EnvVars, err error) {
	env, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	scanner := bufio.NewScanner(bytes.NewReader(env))
	for scanner.Scan() {
		keyVal := strings.SplitN(scanner.Text(), "=", 2)
		if len(keyVal) != 2 {
			continue
		}
		// the value may be quoted (if the file was previously written by WriteEnvironment above)
		val, err := strconv.Unquote(keyVal[1])
		if err != nil {
			vars.Upsert(keyVal[0], keyVal[1])
		} else {
			vars.Upsert(keyVal[0], val)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, trace.Wrap(err)
	}
	return vars, nil
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

// startWebServer starts a web server to serve remote process control on the given listener
// in the specified container.
// This function leaves a running goroutine behind.
func startWebServer(listener net.Listener, c libcontainer.Container) error {
	srv := &http.Server{
		Handler: NewWebServer(c),
	}
	go func() {
		defer func() {
			if err := listener.Close(); err != nil {
				log.Warningf("failed to remove socket file: %v", err)
			}
		}()
		if err := srv.Serve(listener); err != nil {
			log.Infof("server stopped with: %v", err)
		}
	}()
	return nil
}

func serverSockPath(rootfs, socketPath string) string {
	if filepath.IsAbs(socketPath) {
		return socketPath
	}
	return filepath.Join(rootfs, socketPath)
}
