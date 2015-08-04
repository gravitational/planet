package main

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/url"
	"os"

	"github.com/gravitational/cube/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/cube/Godeps/_workspace/src/golang.org/x/net/websocket"
)

func stop(path string) error {
	cfg := ProcessConfig{
		User: "root",
		Args: []string{"/bin/systemctl", "halt"},
	}
	path, err := checkPath(serverSockPath(path), false)
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
