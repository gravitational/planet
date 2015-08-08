package box

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/url"

	"github.com/gravitational/cube/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/cube/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/cube/Godeps/_workspace/src/golang.org/x/net/websocket"
)

type client struct {
	path string
}

func Connect(path string) (ContainerServer, error) {
	path, err := checkPath(serverSockPath(path), false)
	if err != nil {
		return nil, err
	}
	return &client{path: path}, nil
}

func (c *client) Enter(cfg ProcessConfig) error {
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
	conn, err := net.Dial("unix", c.path)
	if err != nil {
		return trace.Wrap(err, "failed to connect to cube socket")
	}
	clt, err := websocket.NewClient(wscfg, conn)
	if err != nil {
		return trace.Wrap(err)
	}
	defer clt.Close()

	exitC := make(chan error, 2)
	go func() {
		_, err := io.Copy(cfg.Out, clt)
		exitC <- err
	}()

	go func() {
		_, err := io.Copy(clt, cfg.In)
		exitC <- err
	}()

	log.Infof("connected to container namespace")

	for i := 0; i < 2; i++ {
		<-exitC
	}
	return nil
}
