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
	"os"
	"path"
	"strings"

	"github.com/containerd/cgroups"

	"github.com/gravitational/trace"
	"github.com/opencontainers/runc/libcontainer"
	libcontainerutils "github.com/opencontainers/runc/libcontainer/utils"
	log "github.com/sirupsen/logrus"
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

// setProcessUserCgroup sets the provided lib-containter process into the /user cgroup inside the container
// this is done opportunistically, so don't cause command execution to fail if setting the cgroup fails
func setProcessUserCgroup(c libcontainer.Container, p *libcontainer.Process) {
	err := setProcessUserCgroupImpl(c, p)
	if err != nil {
		log.Warn("Error setting process into user cgroup: ", trace.DebugReport(err))
	}
}

// setCgroup tries and moves the spawned pid into the cgroup hierarchy for user controlled processes
// the current implementation has a bit of a race condition, if the launched process spawns children before the process
// is moved into the cgroup, the children won't get moved to the correct group. It's not clear to me if there is a better
// runc way to support exec'd processes launching in a separate cgroup than the container itself
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
	// controller
	cgroupPath, ok := state.CgroupPaths["cpu"]
	if !ok {
		return trace.NotFound("cpu cgroup controller not found: %v", state.CgroupPaths)
	}

	if !strings.HasPrefix(cgroupPath, "/sys/fs/cgroup") {
		return trace.BadParameter("Cgroup path not mounted to /sys/fs/cgroup: ", cgroupPath)
	}

	// Example cgroup path: /sys/fs/cgroup/cpu,cpuacct/system.slice/-planet-cee2b8a0-c470-44a6-b7cc-1eefbc1cc88c.scope
	// we want to split off the /sys/fs/cgroup/cpu,cpuacct/ part, and just have the relative cgroup path
	dirs := strings.Split(cgroupPath, "/")

	userPath := path.Join("/", path.Join(dirs[5:]...), "user")

	control, err := cgroups.Load(cgroups.V1, cgroups.StaticPath(userPath))
	if err != nil {
		return trace.Wrap(err)
	}

	return trace.Wrap(control.Add(cgroups.Process{Pid: pid}))
}
