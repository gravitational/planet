/*
Copyright 2018 Gravitational, Inc.

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
	"os"
	"strconv"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/websocket"
)

// ExitError is an error that describes the event of a process exiting with a non-zero value.
type ExitError struct {
	Code int
}

type client struct {
	conn net.Conn
}

// Connect connects to a planet container
func Connect(config *ClientConfig) (ContainerServer, error) {
	notExecutable := false
	socketPath, err := checkPath(config.SocketPath, notExecutable)
	if err != nil {
		return nil, checkError(err)
	}
	socketPath = serverSockPath(config.Rootfs, socketPath)
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
	if cfg.In != nil {
		go func() {
			if _, err := io.Copy(clt, cfg.In); err != nil {
				log.Warnf("Failed to copy input to container: %v.", err)
			}
		}()
	}

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

func dial(socketPath string) (net.Conn, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		if isPermissionDenied(err) {
			return nil, trace.Wrap(err, "Permission denied accessing planet socket. Try using `sudo`?")
		}
		return nil, checkError(trace.Wrap(err, "Failed to connect to planet socket. Check that planet is running."))
	}
	return conn, nil
}

func checkError(err error) error {
	errOrig := trace.Unwrap(err)
	if os.IsNotExist(errOrig) {
		return &ErrConnect{Err: err}
	}
	if _, ok := err.(*net.OpError); ok {
		return &ErrConnect{Err: err}
	}
	return err
}

// IsConnectError returns true if err is a connection error.
func IsConnectError(err error) bool {
	_, ok := err.(*ErrConnect)
	return ok
}

// isPermissionDenied returns true if err is a 'permission denied' error.
func isPermissionDenied(err error) bool {
	if opErr, ok := trace.Unwrap(err).(*net.OpError); ok {
		return opErr.Err != nil && os.IsPermission(opErr.Err)
	}
	return os.IsPermission(trace.Unwrap(err))
}
