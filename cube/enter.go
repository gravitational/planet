package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/url"
	"os"

	"github.com/docker/docker/pkg/term"
	"github.com/gravitational/trace"
	"github.com/opencontainers/runc/libcontainer"
	_ "github.com/opencontainers/runc/libcontainer/nsenter" // this line is important for enter, nothing will work without it
	"golang.org/x/net/websocket"
)

// enter initiates the process in the namespaces of the container
// managed by the cube master process and mantains websocket connection
// to proxy input and output
func enter(path string, cfg ProcessConfig) error {
	oldState, err := term.SetRawTerminal(os.Stdin.Fd())
	if err != nil {
		return err
	}
	defer term.RestoreTerminal(os.Stdin.Fd(), oldState)

	path, err = checkPath(serverSockPath(path), false)
	if err != nil {
		return err
	}
	u := url.URL{Host: "cube", Scheme: "ws", Path: "/v1/enter"}
	data, err := json.Marshal(cfg)
	if err != nil {
		return trace.Wrap(err)
	}
	q := u.Query()
	q.Set("params", hex.EncodeToString(data))
	u.RawQuery = q.Encode()

	wscfg, err := websocket.NewConfig(u.String(), "http://localhost")
	if err != nil {
		return trace.Wrap(err, "failed to enter container")
	}
	conn, err := net.Dial("unix", path)
	if err != nil {
		return trace.Wrap(err, "failed to connect to cube socket")
	}
	c, err := websocket.NewClient(wscfg, conn)
	if err != nil {
		return trace.Wrap(err)
	}

	exitC := make(chan error, 2)
	go func() {
		_, err := io.Copy(os.Stdout, c)
		exitC <- err
	}()

	go func() {
		_, err := io.Copy(c, os.Stdin)
		exitC <- err
	}()

	log.Printf("connected to container namespace")

	for i := 0; i < 2; i++ {
		<-exitC
	}
	return nil
}

type TTY struct {
	W int
	H int
}

type ProcessConfig struct {
	In   io.Reader `json:-`
	Out  io.Writer `json:-`
	TTY  *TTY      `json:"tty"`
	Args []string  `json:"args"`
	User string    `json:"user"`
}

func combinedOutput(c libcontainer.Container, cfg ProcessConfig) ([]byte, error) {
	var b bytes.Buffer
	cfg.Out = &b
	st, err := startProcess(c, cfg)
	if err != nil {
		return b.Bytes(), err
	}
	if !st.Success() {
		return nil, trace.Errorf("process failed with status: %v", st)
	}
	return b.Bytes(), nil
}

func startProcess(c libcontainer.Container, cfg ProcessConfig) (*os.ProcessState, error) {
	log.Printf("start process in the container: %v", cfg)

	if cfg.TTY != nil {
		return startProcessTTY(c, cfg)
	} else {
		return startProcessStdout(c, cfg)
	}

}

func startProcessTTY(c libcontainer.Container, cfg ProcessConfig) (*os.ProcessState, error) {
	log.Printf("start process in the container: %#v", cfg)

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

	log.Printf("started process just okay")

	// wait for the process to finish.
	s, err := p.Wait()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	log.Printf("process status: %v %v", s, err)
	return s, nil
}

func startProcessStdout(c libcontainer.Container, cfg ProcessConfig) (*os.ProcessState, error) {
	log.Printf("start process in the container: %v", cfg)

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

	log.Printf("started process just okay")

	// wait for the process to finish.
	s, err := p.Wait()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	log.Printf("process status: %v %v", s, err)
	return s, nil
}
