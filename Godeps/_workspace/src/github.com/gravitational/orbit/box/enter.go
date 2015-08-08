package box

import (
	"bytes"
	"io"
	"os"

	"github.com/gravitational/cube/Godeps/_workspace/src/github.com/docker/docker/pkg/term"
	"github.com/gravitational/cube/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/cube/Godeps/_workspace/src/github.com/gravitational/trace"

	"github.com/gravitational/cube/Godeps/_workspace/src/github.com/opencontainers/runc/libcontainer"
	_ "github.com/gravitational/cube/Godeps/_workspace/src/github.com/opencontainers/runc/libcontainer/nsenter" // this line is important for enter, nothing will work without it
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
	log.Infof("start process in the container: %v", cfg)

	if cfg.TTY != nil {
		return StartProcessTTY(c, cfg)
	} else {
		return StartProcessStdout(c, cfg)
	}

}

func StartProcessTTY(c libcontainer.Container, cfg ProcessConfig) (*os.ProcessState, error) {
	log.Infof("start process in the container: %#v", cfg)

	p := &libcontainer.Process{
		Args: cfg.Args,
		User: cfg.User,
		Env:  []string{"TERM=xterm"},
	}

	cs, err := p.NewConsole(0)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	term.SetWinsize(cs.Fd(),
		&term.Winsize{Height: uint16(cfg.TTY.H), Width: uint16(cfg.TTY.W)})

	exitC := make(chan error, 2)
	go func() {
		_, err := io.Copy(cfg.Out, cs)
		exitC <- err
	}()

	go func() {
		_, err := io.Copy(cs, cfg.In)
		exitC <- err
	}()

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
	log.Infof("start process in the container: %v", cfg)

	p := &libcontainer.Process{
		Args:   cfg.Args,
		User:   cfg.User,
		Stdout: cfg.Out,
		Stdin:  cfg.In,
		Stderr: cfg.Out,
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
