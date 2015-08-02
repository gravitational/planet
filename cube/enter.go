package main

import (
	"encoding/hex"
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
func enter(p, cmd string) error {
	oldState, err := term.SetRawTerminal(os.Stdin.Fd())
	if err != nil {
		return err
	}
	defer term.RestoreTerminal(os.Stdin.Fd(), oldState)

	path, err := checkPath(serverSockPath(p), false)
	if err != nil {
		return err
	}
	u := url.URL{Host: "cube", Scheme: "ws", Path: "/enter/" + hex.EncodeToString([]byte(cmd))}

	cfg, err := websocket.NewConfig(u.String(), "http://localhost")
	if err != nil {
		return trace.Wrap(err, "failed to enter container")
	}
	conn, err := net.Dial("unix", path)
	if err != nil {
		return trace.Wrap(err, "failed to connect to cube socket")
	}
	c, err := websocket.NewClient(cfg, conn)
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

func startProcess(c libcontainer.Container, args []string, rw io.ReadWriter) error {
	log.Printf("start process in the container: %v", args)

	p := &libcontainer.Process{
		Args: args,
		User: "root",
		Env:  []string{"TERM=xterm"},
	}

	cs, err := p.NewConsole(0)
	if err != nil {
		return trace.Wrap(err)
	}

	term.SetWinsize(cs.Fd(),
		&term.Winsize{Height: uint16(120), Width: uint16(100)})

	exitC := make(chan error, 2)
	go func() {
		_, err := io.Copy(rw, cs)
		exitC <- err
	}()

	go func() {
		_, err := io.Copy(cs, rw)
		exitC <- err
	}()

	// this will cause libcontainer to exec this binary again
	// with "init" command line argument.  (this is the default setting)
	// then our init() function comes into play
	if err := c.Start(p); err != nil {
		return trace.Wrap(err)
	}

	log.Printf("started process just okay")

	// wait for the process to finish.
	s, err := p.Wait()
	if err != nil {
		return trace.Wrap(err)
	}

	log.Printf("process status: %v %v", s, err)
	return err
}
