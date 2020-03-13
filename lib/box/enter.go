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
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/defaults"

	"github.com/containerd/cgroups"
	"github.com/gravitational/trace"
	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/utils"
	"github.com/opencontainers/selinux/go-selinux"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// ExitError is an error that describes the event of a process exiting with a non-zero value.
type ExitError struct {
	// Code specifies the process exit code
	Code int
}

// Error formats the ExitError as a string with the status code
func (err ExitError) Error() string {
	return "exit status " + strconv.FormatInt(int64(err.Code), 10)
}

// CombinedOutput runs a process within planet, returning the output as a byte buffer
func CombinedOutput(config EnterConfig) ([]byte, error) {
	var buf bytes.Buffer
	config.Process.Out = &buf
	err := Enter(config)
	if err != nil {
		return buf.Bytes(), trace.Wrap(err)
	}
	return buf.Bytes(), nil
}

// CombinedOutput runs a process within planet, returning the output as a byte buffer
func (b *Box) CombinedOutput(config ProcessConfig) ([]byte, error) {
	var buf bytes.Buffer
	config.Out = &buf
	err := enter(b.dataDir, b.Container, config)
	if err != nil {
		return buf.Bytes(), trace.Wrap(err)
	}
	return buf.Bytes(), nil
}

// setProcessUserCgroup sets the provided libcontainer process into the /user cgroup inside the container
// this is done on a best effort basis, so we only log if this fails
func setProcessUserCgroup(c libcontainer.Container, p *libcontainer.Process) {
	err := setProcessUserCgroupImpl(c, p)
	if err != nil {
		log.WithError(err).Warn("Error setting process into user cgroup")
	}
}

// setProcessUserCgroupImpl attempts to move the spawned pid into the cgroup hierarchy for user controlled processes
// the current implementation has a bit of a race condition, if the launched process spawns children before the process
// is moved into the cgroup, the children won't get moved to the correct group.
// TODO(knisbet) does runc support a better way of running a process in a separate cgroup from the container itself
func setProcessUserCgroupImpl(c libcontainer.Container, p *libcontainer.Process) error {
	pid, err := p.Pid()
	if err != nil {
		return trace.Wrap(err)
	}

	state, err := c.State()
	if err != nil {
		return trace.Wrap(err)
	}

	// This is a bit of a risk, try and use the cpu controller to identify the cgroup path. CgroupsV1 doesn't use a
	// unified hierarchy, so different controllers can have different cgroup paths. For us, cpu is the most important
	// controller, so we'll use it as the reference
	cgroupPath, ok := state.CgroupPaths["cpu"]
	if !ok {
		return trace.NotFound("cpu cgroup controller not found: %v", state.CgroupPaths)
	}

	if !strings.HasPrefix(cgroupPath, "/sys/fs/cgroup/") {
		return trace.BadParameter("Cgroup path not mounted to /sys/fs/cgroup: %v", cgroupPath)
	}

	// Example cgroup path: /sys/fs/cgroup/cpu,cpuacct/system.slice/-planet-cee2b8a0-c470-44a6-b7cc-1eefbc1cc88c.scope
	// we want to split off the /sys/fs/cgroup/cpu,cpuacct/ part, so we have just the cgroup structure
	// (system.slice/-planet-cee2b8a0-c470-44a6-b7cc-1eefbc1cc88c.scope)
	dirs := strings.Split(cgroupPath, "/")
	if len(dirs) < 6 {
		return trace.BadParameter("cgroup path expected to have atleast 6 directory separators '/'").AddField("cgroup_path", cgroupPath)
	}
	userPath := filepath.Join("/", path.Join(dirs[5:]...), "user")

	control, err := cgroups.Load(cgroups.V1, cgroups.StaticPath(userPath))
	if err != nil {
		return trace.Wrap(err)
	}

	return trace.Wrap(control.Add(cgroups.Process{Pid: pid}))
}

// Enter is used to exec a process within the running container
func Enter(config EnterConfig) error {
	if err := config.checkAndSetDefaults(); err != nil {
		return trace.Wrap(err)
	}
	container, err := getContainer(config.DataDir)
	if err != nil {
		return trace.Wrap(err)
	}
	config.Process.ProcessLabel = config.seLinuxProcessLabel(container.Config().Rootfs)
	return enter(config.DataDir, container, config.Process)
}

func enter(dataDir string, container libcontainer.Container, config ProcessConfig) error {
	// Ensure programs running within the container inherit any proxy settings
	env, err := ReadEnvironment(filepath.Join(container.Config().Rootfs, constants.ProxyEnvironmentFile))
	if err != nil {
		return trace.Wrap(err)
	}
	for _, e := range env {
		if t := config.Env.Get(e.Name); t == "" {
			config.Env.Upsert(e.Name, e.Val)
		}
	}

	p := &libcontainer.Process{
		Args:  config.Args,
		User:  config.User,
		Env:   append(config.Environment(), defaultProcessEnviron()...),
		Label: config.ProcessLabel,
	}

	if config.TTY != nil {
		p.ConsoleHeight = uint16(config.TTY.H)
		p.ConsoleWidth = uint16(config.TTY.W)
	}

	rootuid, err := container.Config().HostRootUID()
	if err != nil {
		return trace.Wrap(err)
	}
	rootgid, err := container.Config().HostRootGID()
	if err != nil {
		return trace.Wrap(err)
	}

	forwarder := NewSignalForwarder()
	var tty *tty
	if config.TTY != nil {
		tty, err = setupTTYIO(p, rootuid, rootgid)
	} else {
		tty, err = setupIO(p, rootuid, rootgid)
	}
	if err != nil {
		return trace.Wrap(err)
	}
	defer tty.Close()

	err = container.Run(p)
	if err != nil {
		return trace.Wrap(err)
	}

	err = tty.waitConsole()
	if err != nil {
		terminate(p)
		return trace.Wrap(err)
	}

	setProcessUserCgroup(container, p)

	err = tty.ClosePostStart()
	if err != nil {
		terminate(p)
		return trace.Wrap(err)
	}

	s, err := forwarder.Forward(p, tty)
	if err != nil {
		terminate(p)
		return trace.Wrap(err)
	}

	log.WithField("status", s).Info("Container process exited.")

	if s != 0 {
		return trace.Wrap(&ExitError{Code: s})
	}
	return nil
}

// EnterConfig specifies the configuration to execute a command inside the container
type EnterConfig struct {
	// Process specifies the process configuration to execute
	Process ProcessConfig
	// DataDir specifies the runc-specific data directory
	DataDir string
	// SELinux specifies whether SELinux support is on
	SELinux bool
}

func (r *EnterConfig) checkAndSetDefaults() error {
	if r.DataDir == "" {
		r.DataDir = defaults.RuncDataDir
	}
	return nil
}

func (r *EnterConfig) seLinuxProcessLabel(rootfs string) (label string) {
	if !selinux.GetEnabled() || !r.SELinux {
		// Empty the label if SELinux has not been enabled
		return ""
	}
	if r.Process.ProcessLabel == "" {
		return getSELinuxProcLabel(rootfs, r.Process.Args[0])
	}
	// Use existing label if specified
	return r.Process.ProcessLabel
}

func getContainer(dataDir string) (libcontainer.Container, error) {
	factory, err := getLibContainerFactory(dataDir)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	absRoot, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	dirs, err := ioutil.ReadDir(absRoot)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if len(dirs) == 0 {
		return nil, trace.NotFound("planet container not found").AddField("data-dir", dataDir)
	}
	var container libcontainer.Container
	var status libcontainer.Status
	for _, dir := range dirs {
		container, err = factory.Load(dir.Name())
		if err != nil {
			if isErrorContainerNotFound(err) {
				log.WithError(err).WithField("dir", dir.Name()).Warn("Container state directory is empty - will skip.")
				continue
			}
			return nil, trace.Wrap(err)
		}

		status, err = container.Status()
		if err != nil {
			return nil, trace.Wrap(err)
		}

		// There should only be a single planet container that's running, so exec within the first
		// running container found
		if status == libcontainer.Running {
			log.WithField("container", container.ID()).Debug("Found running container.")
			break
		}
	}
	if container == nil {
		return nil, trace.NotFound("planet container not found")
	}
	if status == libcontainer.Stopped {
		return nil, trace.BadParameter(errStoppedContainer).AddField("container", container.ID())
	}
	return container, nil
}

func isErrorContainerNotFound(err error) bool {
	errLibc, ok := err.(libcontainer.Error)
	if !ok {
		return false
	}
	return errLibc.Code() == libcontainer.ContainerNotExists
}

func setupTTYIO(process *libcontainer.Process, rootuid, rootgid int) (*tty, error) {
	t := &tty{}

	parent, child, err := utils.NewSockPair("console")
	if err != nil {
		return nil, err
	}

	process.ConsoleSocket = child
	t.postStart = append(t.postStart, parent, child)
	t.consoleC = make(chan error, 1)

	go func() {
		if err := t.recvtty(process, parent); err != nil {
			t.consoleC <- err
		}
		t.consoleC <- nil
	}()

	return t, nil
}

func setupIO(process *libcontainer.Process, rootuid, rootgid int) (*tty, error) {
	i, err := process.InitializeIO(rootuid, rootgid)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	t := &tty{
		closers: []io.Closer{
			i.Stdin,
			i.Stdout,
			i.Stderr,
		},
	}

	// add the process's io to the post start closers if they support close
	for _, cc := range []interface{}{
		process.Stdin,
		process.Stdout,
		process.Stderr,
	} {
		if c, ok := cc.(io.Closer); ok {
			t.postStart = append(t.postStart, c)
		}
	}

	go func() {
		_, _ = io.Copy(i.Stdin, os.Stdin)
		i.Stdin.Close()
	}()
	t.wg.Add(2)
	go t.copyIO(os.Stdout, i.Stdout)
	go t.copyIO(os.Stderr, i.Stderr)

	return t, nil
}

func terminate(p *libcontainer.Process) {
	_ = p.Signal(unix.SIGKILL)
	_, _ = p.Wait()
}

func defaultProcessEnviron() []string {
	return []string{
		"TERM=xterm", "LC_ALL=en_US.UTF-8",
	}
}

const errStoppedContainer = "cannot exec into stopped container. Please start container with `gravity system service start planet` and try again."
