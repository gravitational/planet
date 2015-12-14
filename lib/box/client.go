/*
client.go implements the client interface to Planet box. When a user runs any command, like
"planet stop", it connects to the running instance of itself via a POSIX socket.

It is done via client.Enter(), which executes the command (any process name) and
proxies stdin/stdout to it.
*/
package box

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/url"
	"strconv"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/planet/Godeps/_workspace/src/golang.org/x/net/websocket"
)

// ExitError is an error that describes the event of a process exiting with a non-zero value.
type ExitError struct {
	trace.Traces
	Code int
}

var _ = trace.TraceSetter(&ExitError{})

type client struct {
	conn net.Conn
}

func Connect(config *ClientConfig) (ContainerServer, error) {
	notExecutable := false
	socketPath, err := checkPath(config.SocketPath, notExecutable)
	socketPath = serverSockPath(config.Rootfs, socketPath)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	conn, err := dial(socketPath)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &client{conn: conn}, nil
}

// Enter spawns a process specified with cfg remotely
func (c *client) Enter(cfg ProcessConfig) error {
	u := url.URL{Host: "planet", Scheme: "ws", Path: "/v1/enter"}
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
	clt, err := websocket.NewClient(wscfg, c.conn)
	if err != nil {
		return trace.Wrap(err)
	}
	defer clt.Close()

	// this goroutine copies the output of a container into (usually) stdout,
	// it sends a signal via exitC when it's done (it means the container exited
	// and closed its stdout)
	exitC := make(chan error)
	go pipeClient(cfg.Out, clt, exitC)

	// this goroutine copies stdin into a container. it doesn't exit unless
	// a user hits "Enter" (which causes it to exit io.Copy() loop because it will
	// fail writing to container's closed handle).
	go func() {
		io.Copy(clt, cfg.In)
	}()

	// only wait for output handle to be closed
	err = <-exitC
	return err
}

// pipeClient forwards JSON encoded process output as plain text to dst.
// Upon receiving io.EOF, it terminates and forwards any errors via exitC channel.
func pipeClient(dst io.Writer, conn *websocket.Conn, exitC chan<- error) {
	var err error
	var msg message
	for {
		err = websocket.JSON.Receive(conn, &msg)
		if err != nil {
			break
		}
		_, err = dst.Write(msg.Payload)
		if err != nil {
			break
		}
	}
	if err == io.EOF {
		if msg.ExitCode != 0 {
			err = &ExitError{Code: msg.ExitCode}
		} else {
			err = nil
		}
	}
	exitC <- err
}

func (err ExitError) Error() string {
	return "exit status " + strconv.FormatInt(int64(err.Code), 10)
}

func (err ExitError) OrigError() error {
	return nil
}
