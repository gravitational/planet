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
	"context"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/containerd/cgroups"

	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/trace"
	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/utils"
	libcontainerutils "github.com/opencontainers/runc/libcontainer/utils"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

func CombinedOutput(c libcontainer.Container, cfg ProcessConfig) ([]byte, error) {
	var b bytes.Buffer
	cfg.Out = &b
	err := StartProcess(c, cfg)
	if err != nil {
		return b.Bytes(), err
	}
	return b.Bytes(), nil
}

func StartProcess(c libcontainer.Container, cfg ProcessConfig) error {
	log.Infof("StartProcess(%v)", cfg)
	defer log.Infof("StartProcess(%v) is done!", cfg)

	if cfg.TTY != nil {
		return StartProcessTTY(c, cfg)
	} else {
		return StartProcessStdout(c, cfg)
	}
}

func StartProcessTTY(c libcontainer.Container, cfg ProcessConfig) error {
	p := &libcontainer.Process{
		Args:          cfg.Args,
		User:          cfg.User,
		Env:           append(cfg.Environment(), "TERM=xterm", "LC_ALL=en_US.UTF-8"),
		ConsoleHeight: uint16(cfg.TTY.H),
		ConsoleWidth:  uint16(cfg.TTY.W),
	}

	parentConsole, childConsole, err := libcontainerutils.NewSockPair("console")
	if err != nil {
		return trace.Wrap(err, "failed to create a console socket pair")
	}
	p.ConsoleSocket = childConsole

	// this will cause libcontainer to exec this binary again
	// with "init" command line argument.  (this is the default setting)
	// then our init() function comes into play
	if err := c.Run(p); err != nil {
		return trace.Wrap(err)
	}
	log.Debugf("Process %#v started.", p)

	setProcessUserCgroup(c, p)

	containerConsole, err := getContainerConsole(context.TODO(), parentConsole)
	if err != nil {
		return trace.Wrap(err, "failed to create container console")
	}
	defer containerConsole.Close()

	// start copying output from the process of the container's console
	// into the caller's output:
	if cfg.Out != nil {
		exitC := make(chan error)

		go func() {
			_, err := io.Copy(cfg.Out, containerConsole)
			exitC <- err
		}()
		defer func() {
			<-exitC
		}()
	}

	// start copying caller's input into container's console:
	if cfg.In != nil {
		go io.Copy(containerConsole, cfg.In)
	}

	// wait for the process to finish.
	_, err = p.Wait()
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

func StartProcessStdout(c libcontainer.Container, cfg ProcessConfig) error {
	var in io.Reader
	if cfg.In != nil {
		// we have to pass real pipe to libcontainer.Process because:
		// Libcontainer uses exec.Cmd for entering the master process namespace.
		// In case if standard exec.Cmd gets not a os.File as a parameter
		// to it's Stdin property, it will wait until the read operation
		// will finish in it's Wait method.
		// As long as our web socket never closes on the client side right now
		// this never happens, so this fixes the problem for now
		r, w, err := os.Pipe()
		if err != nil {
			return trace.Wrap(err)
		}
		in = r
		go func() {
			io.Copy(w, cfg.In)
			w.Close()
		}()
	}
	p := &libcontainer.Process{
		Args:   cfg.Args,
		User:   cfg.User,
		Stdout: cfg.Out,
		Stdin:  in,
		Stderr: cfg.Out,
		Env:    append(cfg.Environment(), "TERM=xterm", "LC_ALL=en_US.UTF-8"),
	}

	// this will cause libcontainer to exec this binary again
	// with "init" command line argument.  (this is the default setting)
	// then our init() function comes into play
	if err := c.Start(p); err != nil {
		return trace.Wrap(err)
	}

	setProcessUserCgroup(c, p)

	// wait for the process to finish
	log.Infof("Waiting for StartProcessStdout(%v)...", cfg.Args)
	defer log.Infof("StartProcessStdout(%v) completed", cfg.Args)
	_, err := p.Wait()
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// setProcessUserCgroup sets the provided libcontainer process into the /user cgroup inside the container
// this is done on a best effort basis, so we only log if this fails
func setProcessUserCgroup(c libcontainer.Container, p *libcontainer.Process) {
	err := setProcessUserCgroupImpl(c, p)
	if err != nil {
		log.Warn("Error setting process into user cgroup: ", trace.DebugReport(err))
	}
}

// setProcessUserCgroupImpl tries and moves the spawned pid into the cgroup hierarchy for user controlled processes
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
	userPath := filepath.Join("/", path.Join(dirs[5:]...), "user")

	control, err := cgroups.Load(cgroups.V1, cgroups.StaticPath(userPath))
	if err != nil {
		return trace.Wrap(err)
	}

	return trace.Wrap(control.Add(cgroups.Process{Pid: pid}))
}

func LocalEnter(dataDir string, cfg *ProcessConfig) (int, error) {
	factory, err := getLibContainerFactory(dataDir)
	if err != nil {
		return -1, trace.Wrap(err)
	}

	absRoot, err := filepath.Abs(dataDir)
	if err != nil {
		return -1, trace.Wrap(err)
	}

	list, err := ioutil.ReadDir(absRoot)
	if err != nil {
		return -1, trace.Wrap(err)
	}

	if len(list) == 0 {
		return -1, trace.BadParameter("planet container not found").AddField("data_dir", dataDir)
	}

	var container libcontainer.Container
	var status libcontainer.Status
	for _, fp := range list {
		container, err = factory.Load(fp.Name())
		if err != nil {
			return -1, trace.Wrap(err)
		}

		status, err = container.Status()
		if err != nil {
			return -1, trace.Wrap(err)
		}

		// There should only be a single planet container that's running, so exec within the first
		// running container found
		if status == libcontainer.Running {
			break
		}
	}

	if status == libcontainer.Stopped {
		return -1, trace.BadParameter("cannot exec a container that has stopped")
	}

	// Ensure programs running within the container inheret any proxy settings
	env, err := ReadEnvironment(filepath.Join(container.Config().Rootfs, constants.ProxyEnvironmentFile))
	if err != nil {
		return -1, trace.Wrap(err)
	}
	for _, e := range env {
		if t := cfg.Env.Get(e.Name); t == "" {
			cfg.Env.Upsert(e.Name, e.Val)
		}
	}

	p := &libcontainer.Process{
		Args: cfg.Args,
		User: cfg.User,
		Env:  append(cfg.Environment(), "TERM=xterm", "LC_ALL=en_US.UTF-8"),
	}

	if cfg.TTY != nil {
		p.ConsoleHeight = uint16(cfg.TTY.H)
		p.ConsoleWidth = uint16(cfg.TTY.W)
	}

	rootuid, err := container.Config().HostRootUID()
	if err != nil {
		return -1, trace.Wrap(err)
	}
	rootgid, err := container.Config().HostRootGID()
	if err != nil {
		return -1, trace.Wrap(err)
	}

	forwarder := NewSignalForwarder()
	tty, err := setupIO(p, rootuid, rootgid, cfg.TTY != nil)
	if err != nil {
		return -1, trace.Wrap(err)
	}
	defer tty.Close()

	err = container.Run(p)
	if err != nil {
		return -1, trace.Wrap(err)
	}

	err = tty.waitConsole()
	if err != nil {
		terminate(p)
		return -1, trace.Wrap(err)
	}

	setProcessUserCgroup(container, p)

	err = tty.ClosePostStart()
	if err != nil {
		terminate(p)
		return -1, trace.Wrap(err)
	}

	s, err := forwarder.Forward(p, tty)
	if err != nil {
		terminate(p)
		return -1, trace.Wrap(err)
	}

	logrus.WithField("status", s).Info("container process exited")

	return s, nil
}

func setupIO(process *libcontainer.Process, rootuid, rootgid int, createtty bool) (*tty, error) {
	if createtty {
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

	// not tty access
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
		io.Copy(i.Stdin, os.Stdin)
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
