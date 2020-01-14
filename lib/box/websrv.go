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

package box

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os/exec"
	"syscall"

	"github.com/gravitational/planet/lib/constants"

	"github.com/gravitational/roundtrip"
	"github.com/gravitational/trace"

	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/websocket"
)

// commandOutput is an io.Writer on server-side of the websocket-based remote
// command execution protocol that forwards process output and exit code
// to client as JSON.
type commandOutput struct {
	conn *websocket.Conn
}

// message is a piece of process output and optionally an exit code.
type message struct {
	Payload  []byte `json:"payload"`
	ExitCode int    `json:"exit_code,omitempty"`
}

type handler func(http.ResponseWriter, *http.Request, httprouter.Params) error

type webServer struct {
	httprouter.Router
	socketServer websocket.Server
	box          *Box
}

// NewWebServer creates a new instance of the web server that
// handles external command execution requests
func NewWebServer(box *Box) *webServer {
	s := &webServer{box: box}

	// it has to be GET because we use websockets,
	// so we are using the weird argument passing in query
	// string here
	s.GET("/v1/enter", s.makeJSONHandler(s.enter))
	return s
}

// makeJsonHandler wraps a standard HTTP handler and adds unified
// error checking and JSON encoding of the output.
func (h *webServer) makeJSONHandler(fn handler) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		err := fn(w, r, p)
		if err == nil {
			return
		}
		log.WithError(err).Error("Handler failed.")
		roundtrip.ReplyJSON(w, http.StatusInternalServerError, err.Error())
	}
}

// enter is the handler for HTTP GET /v1/enter
func (s *webServer) enter(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	q := r.URL.Query()
	params, err := hex.DecodeString(q.Get("params"))
	if err != nil {
		return trace.Wrap(err)
	}

	var cfg *ProcessConfig
	if err := json.Unmarshal(params, &cfg); err != nil {
		return trace.Wrap(err)
	}

	log.WithField("process", cfg).Debug("Enter.")

	// use websocket server to establish a bidirectional communication:
	s.socketServer.Handler = func(conn *websocket.Conn) {
		defer conn.Close()
		var err error
		cmdOut := &commandOutput{conn: conn}

		cfg.In = conn
		cfg.Out = cmdOut

		err = s.box.StartProcess(*cfg)
		if err == nil {
			return
		}

		log.WithFields(log.Fields{
			log.ErrorKey: err,
			"process":    cfg,
		}).Warn("StartProcess failed.")

		if errExit, ok := trace.Unwrap(err).(*exec.ExitError); ok {
			if waitStatus, ok := errExit.ProcessState.Sys().(syscall.WaitStatus); ok {
				cmdOut.writeExitCode(waitStatus.ExitStatus())
				return
			}
		}
		cmdOut.writeExitCode(constants.ExitCodeUnknown)
	}

	s.socketServer.ServeHTTP(w, r)
	return nil
}

func (r *commandOutput) Write(p []byte) (n int, err error) {
	err = r.writeMessage(&message{
		Payload: p,
	})
	return len(p), err
}

func (r *commandOutput) writeMessage(msg *message) error {
	err := websocket.JSON.Send(r.conn, msg)
	return err
}

func (r *commandOutput) writeExitCode(exitCode int) error {
	err := r.writeMessage(&message{
		ExitCode: exitCode,
	})
	return err
}
