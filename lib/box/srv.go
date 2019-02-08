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
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/gravitational/go-udev"
	"github.com/gravitational/trace"

	log "github.com/sirupsen/logrus"

	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/cgroups/systemd"
	"github.com/opencontainers/runc/libcontainer/configs"
	"github.com/pborman/uuid"
)

type ContainerServer interface {
	Enter(cfg ProcessConfig) error
}

// Box defines a running planet container.
//
// A box manages a number of resources including an init process
// and an API server that exposes a unix socket endpoint.
// Once started, the box can be shut down with Close.
type Box struct {
	Process   *libcontainer.Process
	Container libcontainer.Container
	listener  net.Listener
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

// Wait blocks waiting the init process to finish.
// Returns the state of the init process.
func (b *Box) Wait() (*os.ProcessState, error) {
	log.Infof("box.Wait() is called")
	st, err := b.Process.Wait()
	if e, ok := err.(*exec.ExitError); ok {
		return e.ProcessState, nil
	}
	return st, err
}

// Starts the container described by cfg.
// Returns a Box instance or an error.
func Start(cfg Config) (*Box, error) {
	log.Infof("[BOX] starting with config: %#v", cfg)

	if !systemd.UseSystemd() {
		return nil, trace.BadParameter("unable to use systemd for container creation")
	}

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

	// Resolve the paths for {newuidmap,newgidmap} from the context of runc,
	// to avoid doing a path lookup in the nsexec context.
	// TODO: the binary names are not currently configurable.
	newuidmap, err := exec.LookPath("newuidmap")
	if err != nil {
		newuidmap = ""
	}
	newgidmap, err := exec.LookPath("newgidmap")
	if err != nil {
		newgidmap = ""
	}

	root, err := libcontainer.New(cfg.DataDir,
		libcontainer.SystemdCgroups,
		libcontainer.NewuidmapPath(newuidmap),
		libcontainer.NewgidmapPath(newgidmap),
	)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	containerID := uuid.New()
	// https://github.com/systemd/systemd/blob/9ed6a1d2c6fd60ea666a30f815ab4c776e5d5c7c/src/core/machine-id-setup.c#L54-L58
	// Pass the container uuid to systemd init
	cfg.InitEnv = append(cfg.InitEnv, fmt.Sprintf("container_uuid=%v", containerID))

	config, err := getLibcontainerConfig(containerID, rootfs, cfg)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// Bootstrap the container.
	//
	//
	// In this step, runc will execute the special "planet init" command
	// that will set things up for the container:
	//
	// The command runs nsexec code required to initialize namespaces _before_
	// the Go runtime.
	//
	// Then it runs the factory.StartInitialization to set up the container.
	//
	// After all of the above is done, "planet init" will block waiting for execve
	// to start the container's init process (see container.Start below).
	container, err := root.Create(containerID, config)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	defer func() {
		if err != nil {
			container.Destroy()
		}
	}()

	// start the API server
	socketPath := serverSockPath(cfg.Rootfs, cfg.SocketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	defer func() {
		if err != nil {
			listener.Close()
		}
	}()

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
		Init:   true,
		Cwd:    "/",
	}

	// Run the container by starting the init process.
	if err := container.Run(process); err != nil {
		return nil, trace.Wrap(err)
	}

	status, err := container.Status()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	log.Infof("Container status: %v (err=%v).", status, err)

	return &Box{
		Process:   process,
		Container: container,
		listener:  listener,
	}, nil
}

func getLibcontainerConfig(containerID, rootfs string, cfg Config) (*configs.Config, error) {
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

	// find all existing loop devices on a host and re-create them inside the container
	hostLoops, err := filepath.Glob("/dev/loop?")
	if err != nil {
		return nil, trace.Wrap(err)
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
			// Mount the cgroup filesystem into the container
			/*{
				Source:      "cgroup",
				Destination: "/sys/fs/cgroup",
				Device:      "cgroup",
				Flags:       defaultMountFlags,
			},*/
		},
		Cgroups: &configs.Cgroup{
			Name: fmt.Sprintf("planet-%v", containerID),
			Resources: &configs.Resources{
				AllowAllDevices:  &allowAllDevices,
				AllowedDevices:   configs.DefaultAllowedDevices,
				MemorySwappiness: nil, // nil means "machine-default" and that's what we need because we don't care
			},
		},

		Devices:  append(configs.DefaultAutoCreatedDevices, append(loopDevices, disks...)...),
		Hostname: hostname,
	}

	// iterate over all the loaded cgroup controllers, and mount them inside the container
	hostCgroups, err := parseHostCgroups()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	for controller, _ := range hostCgroups {
		mount := &configs.Mount{
			Source:      "cgroup",
			Destination: path.Join("/sys/fs/cgroup", controller),
			Device:      "cgroup",
			Flags:       syscall.MS_NOSUID | syscall.MS_NOEXEC | syscall.MS_NODEV | syscall.MS_STRICTATIME,
			Data:        controller,
		}
		config.Mounts = append(config.Mounts, mount)
	}

	for _, mountSpec := range cfg.Mounts {
		matches, err := filepath.Glob(mountSpec.Src)
		if err != nil {
			return nil, trace.Wrap(err, "invalid glob pattern %q", mountSpec.Src)
		}

		if len(matches) == 0 && mountSpec.SkipIfMissing {
			// Skip the non-existent mount source
			continue
		}

		for _, match := range matches {
			targetPath := mountSpec.Dst
			if match != mountSpec.Src {
				// For glob patterns, targetPath implicitly equals the source
				targetPath = match
			}
			mount := &configs.Mount{
				Device:      "bind",
				Source:      match,
				Destination: targetPath,
				Flags:       syscall.MS_BIND,
			}
			if mountSpec.Readonly {
				mount.Flags |= syscall.MS_RDONLY
			}
			if mountSpec.Recursive {
				mount.Flags |= syscall.MS_REC
			}
			config.Mounts = append(config.Mounts, mount)
		}
	}

	for _, d := range cfg.Devices {
		devices, err := convertToDevices(d)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		config.Devices = append(config.Devices, devices...)
	}

	return config, nil
}

func convertToDevices(device Device) (devices []*configs.Device, err error) {
	// each device path passed on CLI is treated as a glob
	devicePaths, err := filepath.Glob(device.Path)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	for _, devicePath := range devicePaths {
		deviceInfo, err := getDeviceInfo(devicePath)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		// fill in other fields from the parameters passed on CLI
		deviceInfo.Permissions = device.Permissions
		deviceInfo.FileMode = device.FileMode
		deviceInfo.Uid = device.UID
		deviceInfo.Gid = device.GID
		devices = append(devices, deviceInfo)
	}
	return devices, nil
}

func getDeviceInfo(devicePath string) (*configs.Device, error) {
	stat := syscall.Stat_t{}
	if err := syscall.Stat(devicePath, &stat); err != nil {
		return nil, trace.Wrap(err)
	}
	// determine device type, char or block
	var deviceType rune
	switch stat.Mode & syscall.S_IFMT {
	case syscall.S_IFBLK:
		deviceType = 'b'
	case syscall.S_IFCHR:
		deviceType = 'c'
	default:
		return nil, trace.BadParameter("unsupported device type: %q", devicePath)
	}
	return &configs.Device{
		Type:  deviceType,
		Path:  devicePath,
		Major: int64(stat.Rdev / 256),
		Minor: int64(stat.Rdev % 256),
	}, nil
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

func checkPath(path string, executable bool) (absPath string, err error) {
	if path == "" {
		return "", trace.BadParameter("path to root filesystem can not be empty")
	}
	absPath, err = filepath.Abs(path)
	if err != nil {
		return "", trace.Wrap(err)
	}
	fi, err := os.Stat(absPath)
	if err != nil {
		return "", trace.ConvertSystemError(err)
	}
	if executable && (fi.Mode()&0111 == 0) {
		return "", trace.BadParameter("file %v is not executable", absPath)
	}
	return absPath, nil
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
				log.Warnf("Failed to remove socket file: %v.", err)
			}
		}()
		if err := srv.Serve(listener); err != nil {
			log.Warnf("Server finished with %v.", err)
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
