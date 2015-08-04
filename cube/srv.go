package main

import (
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/gravitational/roundtrip"
	"github.com/gravitational/trace"

	"github.com/julienschmidt/httprouter"
	"github.com/opencontainers/runc/libcontainer"

	"golang.org/x/net/websocket"
)

type ContainerServer struct {
	httprouter.Router
	c libcontainer.Container
}

func NewServer(c libcontainer.Container) *ContainerServer {
	s := &ContainerServer{
		c: c,
	}

	// it has to be GET because we use websockets,
	// so we are using the weird argument passing in query
	// string here
	s.GET("/v1/enter", s.handle(s.enter))
	return s
}

func (h *ContainerServer) handle(fn handler) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		if err := fn(w, r, p); err != nil {
			log.Printf("error in handler: %v", err)
			roundtrip.ReplyJSON(
				w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	return nil
}

func (s *ContainerServer) enter(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	q := r.URL.Query()

	log.Printf("query: %v", q)

	params, err := hex.DecodeString(q.Get("params"))
	if err != nil {
		return trace.Wrap(err)
	}

	var cfg *ProcessConfig
	if err := json.Unmarshal(params, &cfg); err != nil {
		return trace.Wrap(err)
	}

	log.Printf("entering command: %v", cfg)
	ws := &enterHandler{
		cfg: *cfg,
		c:   s.c,
	}
	defer ws.Close()
	ws.Handler().ServeHTTP(w, r)
	return nil
}

type enterHandler struct {
	cfg ProcessConfig
	c   libcontainer.Container
}

func (w *enterHandler) Close() error {
	log.Printf("enterHandler.Close()")
	return nil
}

func (w *enterHandler) handle(ws *websocket.Conn) {
	defer ws.Close()
	status, err := w.enter(ws)
	if err != nil {
		log.Printf("enter error: %v", err)
	}
	log.Printf("process ended with status: %v", status)
}

func (w *enterHandler) enter(ws *websocket.Conn) (*os.ProcessState, error) {
	log.Printf("start process in a container: %v", w.cfg)

	defer ws.Close()
	w.cfg.In = ws
	w.cfg.Out = ws
	return startProcess(w.c, w.cfg)
}

func (w *enterHandler) Handler() http.Handler {
	// TODO(klizhentas)
	// we instantiate a server explicitly here instead of using
	// websocket.HandlerFunc to set empty origin checker
	// make sure we check origin when in prod mode
	return &websocket.Server{
		Handler: w.handle,
	}
}

type handler func(http.ResponseWriter, *http.Request, httprouter.Params) error
