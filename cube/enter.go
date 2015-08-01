package main

import (
	"encoding/hex"
	"io"
	"log"
	"net"
	"net/url"
	"os"

	"github.com/gravitational/trace"

	"golang.org/x/net/websocket"
)

func enter(p, cmd string) error {
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
