package box

import (
	"bytes"
	"io"
	"os"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/docker/docker/pkg/term"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/opencontainers/runc/libcontainer"
	_ "github.com/gravitational/planet/Godeps/_workspace/src/github.com/opencontainers/runc/libcontainer/nsenter" // this line is important for enter, nothing will work without it
)

func CombinedOutput(c libcontainer.Container, cfg ProcessConfig) ([]byte, error) {
	var b bytes.Buffer
	cfg.Out = &b
	st, err := StartProcess(c, cfg)
	if err != nil {
		return b.Bytes(), err
	}
	if !st.Success() {
		return nil, trace.Errorf("process failed with status: %v", st)
	}
	return b.Bytes(), nil
}

func StartProcess(c libcontainer.Container, cfg ProcessConfig) (*os.ProcessState, error) {
	log.Infof("StartProcess(%v)", cfg)
	defer log.Infof("StartProcess(%v) is gone!", cfg)

	if cfg.TTY != nil {
		return StartProcessTTY(c, cfg)
	} else {
		return StartProcessStdout(c, cfg)
	}
}

func StartProcessTTY(c libcontainer.Container, cfg ProcessConfig) (*os.ProcessState, error) {
	p := &libcontainer.Process{
		Args: cfg.Args,
		User: cfg.User,
		Env:  []string{"TERM=xterm"},
	}

	containerConsole, err := p.NewConsole(0)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	term.SetWinsize(containerConsole.Fd(),
		&term.Winsize{Height: uint16(cfg.TTY.H), Width: uint16(cfg.TTY.W)})

	// start copying output from the process of the container's console
	// into the caller's output:
	if cfg.Out != nil {
		go func() {
			io.Copy(cfg.Out, containerConsole)
		}()
	}

	// start copying caller's input into container's console:
	if cfg.In != nil {
		go func() {
			io.Copy(containerConsole, cfg.In)
		}()
	}

	// this will cause libcontainer to exec this binary again
	// with "init" command line argument.  (this is the default setting)
	// then our init() function comes into play
	if err := c.Start(p); err != nil {
		return nil, trace.Wrap(err)
	}

	log.Infof("started process just okay")

	// wait for the process to finish.
	s, err := p.Wait()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	log.Infof("process status: %v %v", s, err)
	return s, nil
}

func StartProcessStdout(c libcontainer.Container, cfg ProcessConfig) (*os.ProcessState, error) {
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
			return nil, trace.Wrap(err)
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
	}

	// this will cause libcontainer to exec this binary again
	// with "init" command line argument.  (this is the default setting)
	// then our init() function comes into play
	if err := c.Start(p); err != nil {
		return nil, trace.Wrap(err)
	}

	// wait for the process to finish
	log.Infof("Waiting for StartProcessStdout(%v)...", cfg.Args)
	defer log.Infof("StartProcessStdout(%v) completed", cfg.Args)
	s, err := p.Wait()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return s, nil
}
