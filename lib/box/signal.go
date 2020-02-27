/*
Copyright 2020 Gravitational, Inc.

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

// Based on: https://github.com/opencontainers/runc/blob/master/signals.go

package box

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/gravitational/trace"
	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/utils"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

type signalForwarder struct {
	signalC chan os.Signal
}

// NewsignalForwarder creates a default signalForwarder
func NewSignalForwarder() signalForwarder {
	s := make(chan os.Signal, 128)
	signal.Notify(s)
	return signalForwarder{
		signalC: s,
	}
}

// Forward sets up signal handling from the current process and forwarding to the child process running inside the
// container
func (s *signalForwarder) Forward(process *libcontainer.Process, tty *tty) (int, error) {
	pid, err := process.Pid()
	if err != nil {
		return -1, trace.Wrap(err)
	}

	// Perform the initial tty resize. Always ignore errors resizing because
	// stdout might have disappeared (due to races with when SIGHUP is sent).
	_ = tty.resize()

	for signal := range s.signalC {
		switch signal {
		case unix.SIGWINCH:
			// Ignore errors resizing, as above.
			_ = tty.resize()
		case unix.SIGCHLD:
			exits, err := s.reap()
			if err != nil {
				logrus.WithError(err).Error("error reaping child")
			}
			for _, e := range exits {
				logrus.WithFields(logrus.Fields{
					"pid":    e.pid,
					"status": e.status,
				}).Debug("process exited")
				if e.pid == pid {
					// call Wait() on the process even though we already have the exit
					// status because we must ensure that any of the go specific process
					// fun such as flushing pipes are complete before we return.
					_, _ = process.Wait()
					return e.status, nil
				}
			}
		default:
			logrus.WithFields(logrus.Fields{
				"pid":    pid,
				"signal": signal,
			}).Debug("sending signal to process")
			if err := unix.Kill(pid, signal.(syscall.Signal)); err != nil {
				logrus.Error(err)
			}
		}
	}

	return -1, nil
}

// exit models a process exit status with the pid and
// exit status.
type exit struct {
	pid    int
	status int
}

// reap runs wait4 in a loop until we have finished processing any existing exits
// then returns all exits to the main event loop for further processing.
func (h *signalForwarder) reap() (exits []exit, err error) {
	var (
		ws  unix.WaitStatus
		rus unix.Rusage
	)
	for {
		pid, err := unix.Wait4(-1, &ws, unix.WNOHANG, &rus)
		if err != nil {
			if err == unix.ECHILD {
				return exits, nil
			}
			return nil, trace.Wrap(err)
		}
		if pid <= 0 {
			return exits, nil
		}
		exits = append(exits, exit{
			pid:    pid,
			status: utils.ExitStatus(ws),
		})
	}
}
