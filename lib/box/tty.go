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

// Based on: https://github.com/opencontainers/runc/blob/master/tty.go

package box

import (
	"io"
	"os"
	"os/signal"
	"sync"

	"github.com/containerd/console"
	"github.com/gravitational/trace"
	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/utils"
	"golang.org/x/sys/unix"
)

type tty struct {
	epoller   *console.Epoller
	console   *console.EpollConsole
	stdin     console.Console
	closers   []io.Closer
	postStart []io.Closer
	wg        sync.WaitGroup
	consoleC  chan error
}

func (t *tty) recvtty(process *libcontainer.Process, socket *os.File) (Err error) {
	f, err := utils.RecvFd(socket)
	if err != nil {
		return trace.Wrap(err)
	}
	cons, err := console.ConsoleFromFile(f)
	if err != nil {
		return trace.Wrap(err)
	}
	err = console.ClearONLCR(cons.Fd())
	if err != nil {
		return trace.Wrap(err)
	}
	epoller, err := console.NewEpoller()
	if err != nil {
		return trace.Wrap(err)
	}
	epollConsole, err := epoller.Add(cons)
	if err != nil {
		return trace.Wrap(err)
	}
	defer func() {
		if Err != nil {
			epollConsole.Close()
		}
	}()
	go epoller.Wait()
	go io.Copy(epollConsole, os.Stdin)
	t.wg.Add(1)
	go t.copyIO(os.Stdout, epollConsole)

	// set raw mode to stdin and also handle interrupt
	stdin, err := console.ConsoleFromFile(os.Stdin)
	if err != nil {
		return trace.Wrap(err)
	}
	if err := stdin.SetRaw(); err != nil {
		return trace.Wrap(err, "failed to set the terminal from the stdin")
	}
	go handleInterrupt(stdin)

	t.epoller = epoller
	t.stdin = stdin
	t.console = epollConsole
	t.closers = []io.Closer{epollConsole}
	return nil
}

func handleInterrupt(c console.Console) {
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, os.Interrupt)
	<-sigchan
	_ = c.Reset()
	os.Exit(0)
}

func (t *tty) resize() error {
	if t.console == nil {
		return nil
	}
	return t.console.ResizeFrom(console.Current())
}

func (t *tty) copyIO(w io.Writer, r io.ReadCloser) {
	defer t.wg.Done()
	io.Copy(w, r)
	r.Close()
}

// Close closes all open fds for the tty and/or restores the original
// stdin state to what it was prior to the container execution
func (t *tty) Close() error {
	// ensure that our side of the fds are always closed
	for _, c := range t.postStart {
		c.Close()
	}
	// the process is gone at this point, shutting down the console if we have
	// one and wait for all IO to be finished
	if t.console != nil && t.epoller != nil {
		t.console.Shutdown(t.epoller.CloseConsole)
	}
	t.wg.Wait()
	for _, c := range t.closers {
		c.Close()
	}
	if t.stdin != nil {
		t.stdin.Reset()
	}
	return nil
}

func (t *tty) waitConsole() error {
	if t.consoleC != nil {
		return <-t.consoleC
	}
	return nil
}

// ClosePostStart closes any fds that are provided to the container and dup2'd
// so that we no longer have copy in our process.
func (t *tty) ClosePostStart() error {
	for _, c := range t.postStart {
		c.Close()
	}
	return nil
}

// Winsize represents the size of the terminal window.
type Winsize struct {
	Height uint16
	Width  uint16
	x      uint16
	y      uint16
}

// GetWinsize returns the window size based on the specified file descriptor.
func GetWinsize(fd uintptr) (*Winsize, error) {
	uws, err := unix.IoctlGetWinsize(int(fd), unix.TIOCGWINSZ)
	ws := &Winsize{Height: uws.Row, Width: uws.Col, x: uws.Xpixel, y: uws.Ypixel}
	return ws, err
}
