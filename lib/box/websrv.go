package box

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os/exec"
	"syscall"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/roundtrip"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/julienschmidt/httprouter"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/opencontainers/runc/libcontainer"

	"github.com/gravitational/planet/Godeps/_workspace/src/golang.org/x/net/websocket"
)

// commandOutput is an io.Writer on server-side of the websocket-based remote
// command execution protocol that forwards process output and exit code
// to client
type commandOutput struct {
	enc *json.Encoder
}

// Message is a piece of process output and optionally an exit code
type Message struct {
	Payload  string `json:"payload"`
	ExitCode int    `json:"exitCode,omitempty"`
}

type handler func(http.ResponseWriter, *http.Request, httprouter.Params) error

type webServer struct {
	httprouter.Router
	container    libcontainer.Container
	socketServer websocket.Server
}

func NewWebServer(c libcontainer.Container) *webServer {
	s := &webServer{container: c}

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
		if err := fn(w, r, p); err != nil {
			log.Errorf("error in handler: %v", err)
			roundtrip.ReplyJSON(
				w, http.StatusInternalServerError, err.Error())
			return
		}
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

	log.Infof("webServer.enter(command=%v)", cfg)

	// use websocket server to establish a bidirectional communication:
	s.socketServer.Handler = func(conn *websocket.Conn) {
		defer conn.Close()
		var err error
		cmdOut := &commandOutput{enc: json.NewEncoder(conn)}

		cfg.In = conn
		cfg.Out = cmdOut
		if err = StartProcess(s.container, *cfg); err != nil {
			log.Errorf("StartProcess failed with %v", err)
			if errTrace, ok := err.(*trace.TraceErr); ok {
				if errExit, ok := errTrace.OrigError().(*exec.ExitError); ok {
					if waitStatus, ok := errExit.ProcessState.Sys().(syscall.WaitStatus); ok {
						cmdOut.writeExitCode(waitStatus.ExitStatus())
					}
				}
			}
		}
		log.Infof("StartProcess (%v) completed!", cfg)
	}
	s.socketServer.ServeHTTP(w, r)
	return nil
}

func (r *commandOutput) Write(p []byte) (n int, err error) {
	return r.writeMessage(&Message{
		Payload: string(p),
	})
}

func (r *commandOutput) writeMessage(msg *Message) (n int, err error) {
	err = r.enc.Encode(msg)
	return len(msg.Payload), err
}

func (r *commandOutput) writeExitCode(exitCode int) (err error) {
	_, err = r.writeMessage(&Message{
		ExitCode: exitCode,
	})
	return err
}
