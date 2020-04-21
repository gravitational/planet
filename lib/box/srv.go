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
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/defaults"

	"github.com/gravitational/go-udev"
	"github.com/gravitational/trace"
	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/cgroups/systemd"
	"github.com/opencontainers/runc/libcontainer/configs"
	"github.com/pborman/uuid"
	log "github.com/sirupsen/logrus"
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
	*libcontainer.Process
	libcontainer.Container
	listener net.Listener
	config   Config
	// dataDir specifies the runc-specific data location on host
	dataDir string
}

// Close shuts down the box. It is written to be safe to call multiple
// times in a row for extra robustness.
func (b *Box) Close() error {
	var errors []error
	if b.Container != nil {
		if err := b.Container.Destroy(); err != nil {
			errors = append(errors, err)
		}
	}
	if b.listener != nil {
		if err := b.listener.Close(); err != nil {
			errors = append(errors, err)
		}
	}
	return trace.NewAggregate(errors...)
}

// Wait blocks waiting the init process to finish.
// Returns the state of the init process.
func (b *Box) Wait() (*os.ProcessState, error) {
	b.config.Info("Wait.")
	state, err := b.Process.Wait()
	if err, ok := err.(*exec.ExitError); ok {
		return err.ProcessState, nil
	}
	return state, err
}

func getLibContainerFactory(dataDir string) (libcontainer.Factory, error) {
	if !systemd.UseSystemd() {
		return nil, trace.BadParameter("unable to use systemd for container creation")
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

	factory, err := libcontainer.New(dataDir,
		libcontainer.SystemdCgroups,
		libcontainer.NewuidmapPath(newuidmap),
		libcontainer.NewgidmapPath(newgidmap),
	)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return factory, nil
}

// Starts the container described by cfg.
// Returns a Box instance or an error.
func Start(cfg Config) (*Box, error) {
	if err := cfg.checkAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}

	cfg.WithField("config", fmt.Sprintf("%#v", cfg)).Info("Start.")

	rootfs, err := checkPath(cfg.Rootfs, false)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	if len(cfg.EnvFiles) != 0 {
		for _, ef := range cfg.EnvFiles {
			cfg.WithField("env-file", ef.Env).Infof("Write environment file.")
			if err := WriteEnvironment(filepath.Join(rootfs, ef.Path), ef.Env); err != nil {
				return nil, trace.Wrap(err)
			}
		}
	}

	if len(cfg.Files) != 0 {
		for _, f := range cfg.Files {
			path := filepath.Join(rootfs, f.Path)
			cfg.WithField("file", path).Infof("Write file.")
			if err := writeFile(path, f); err != nil {
				return nil, trace.Wrap(err)
			}
		}
	}

	root, err := getLibContainerFactory(cfg.DataDir)
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
			err := container.Destroy()
			if err != nil {
				cfg.WithError(err).Warn("Failed to destroy container.")
			}
		}
	}()

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
	err = container.Run(process)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	status, err := container.Status()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	cfg.WithFields(log.Fields{
		log.ErrorKey: err,
		"status":     status,
	}).Info("Start container.")

	box := &Box{
		Process:   process,
		Container: container,
		config:    cfg,
	}
	return box, nil
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
				Permissions: constants.DeviceReadWritePerms,
				FileMode:    constants.GroupReadWriteMask,
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
			Permissions: constants.DeviceReadWritePerms,
			FileMode:    constants.GroupReadWriteMask,
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
				Source:      "proc",
				Destination: "/proc",
				Device:      "proc",
				Flags:       defaultMountFlags,
			},
			// this is needed for flanneld that does modprobe
			{
				Source:      "/lib/modules",
				Destination: "/lib/modules",
				Device:      "bind",
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
				Flags:       defaultMountFlags,
			},
			{
				Source:      "devpts",
				Destination: "/dev/pts",
				Device:      "devpts",
				Flags:       syscall.MS_NOSUID | syscall.MS_NOEXEC,
				Data:        "newinstance,ptmxmode=0666,mode=0620,gid=5",
			},
			{
				Source:      "shm",
				Destination: "/dev/shm",
				Device:      "tmpfs",
				Data:        "mode=1777,size=65536k",
				Flags:       defaultMountFlags,
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
			// kernel printk buffer is needed to give access to the
			// kernel log to tools like node-problem-detector
			{
				Device:      "bind",
				Source:      "/dev/kmsg",
				Destination: "/dev/kmsg",
				Flags:       syscall.MS_BIND | syscall.MS_RDONLY,
			},
			{
				Source:      "tmpfs",
				Destination: "/run",
				Device:      "tmpfs",
				Flags:       defaultMountFlags,
			},
			{
				Source:      "tmpfs",
				Destination: "/run/lock",
				Device:      "tmpfs",
				Flags:       defaultMountFlags,
			},
			// /run has to be mounted explicitly as tmpfs in order to be able
			// to mount /run/udev below
			{
				Source:      "tmpfs",
				Destination: "/run",
				Device:      "tmpfs",
				Flags:       syscall.MS_NOSUID | syscall.MS_NODEV,
				Data:        "mode=755",
			},
			// /run/udev is used by OpenEBS node device manager to detect
			// added and removed block devices
			{
				Device:      "bind",
				Source:      "/run/udev",
				Destination: "/run/udev",
				Flags:       syscall.MS_BIND,
			},
		},
		Cgroups: &configs.Cgroup{
			Name: fmt.Sprintf("planet-%v", containerID),
			Resources: &configs.Resources{
				AllowAllDevices:  &allowAllDevices,
				AllowedDevices:   configs.DefaultAllowedDevices,
				MemorySwappiness: nil, // nil means "machine-default" and that's what we need because we don't care
				CpuShares:        2,   // set planet to minimum cpu shares relative to host services
				PidsLimit:        -1,  // override systemd defaults and set planet scope to unlimitted pids
			},
		},
		Devices:  append(configs.DefaultAutoCreatedDevices, append(loopDevices, disks...)...),
		Hostname: hostname,
	}
	if cfg.SELinux {
		config.MountLabel = defaults.ContainerFileLabel
		config.ProcessLabel = cfg.ProcessLabel
		config.Mounts = append(config.Mounts, []*configs.Mount{
			{
				Destination: "/sys/fs/selinux",
				Source:      "/sys/fs/selinux",
				Device:      "bind",
				Flags:       syscall.MS_BIND | syscall.MS_RELATIME,
			},
			{
				Device:      "bind",
				Source:      "/etc/selinux",
				Destination: "/etc/selinux",
				Flags:       defaultMountFlags | syscall.MS_BIND | syscall.MS_RDONLY,
			},
		}...)
	}

	// Cgroup namespaces aren't currently available in redhat/centos based kernels
	// only use cgroup namespaces on kernels that have cgroup namespaces enabled
	cgroupsEnabled, err := CgroupNSEnabled()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if cgroupsEnabled {
		config.Namespaces = append(config.Namespaces, configs.Namespace{
			Type: configs.NEWCGROUP,
		})
		config.Mounts = append(config.Mounts, &configs.Mount{
			Source:      "cgroup",
			Destination: "/sys/fs/cgroup",
			Device:      "cgroup",
			Flags:       syscall.MS_PRIVATE,
		})
	} else {
		config.Mounts = append(config.Mounts, &configs.Mount{
			Source:      "/sys/fs/cgroup",
			Destination: "/sys/fs/cgroup",
			Device:      "bind",
			Flags:       syscall.MS_PRIVATE | syscall.MS_BIND,
		})
	}

	for _, mountSpec := range cfg.Mounts {
		matches, err := filepath.Glob(mountSpec.Src)
		if err != nil {
			return nil, trace.Wrap(err, "invalid glob pattern %q", mountSpec.Src)
		}

		if len(matches) == 0 {
			if !mountSpec.SkipIfMissing {
				return nil, trace.NotFound("mount source not found").AddFields(map[string]interface{}{
					"src": mountSpec.Src,
					"dst": mountSpec.Dst,
				})
			}
			// Skip the non-existent mount source if SkipIfMissing is set
			continue
		}

		for _, match := range matches {
			targetPath := mountSpec.Dst
			if match != mountSpec.Src {
				// For glob patterns, targetPath implicitly equals the source
				targetPath = match
			}
			mount := &configs.Mount{
				Device:           "bind",
				Source:           match,
				Destination:      targetPath,
				Flags:            syscall.MS_BIND,
				PropagationFlags: []int{syscall.MS_SHARED},
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
