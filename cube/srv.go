package main

import (
	"encoding/hex"
	"log"
	"net/http"

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

	s.GET("/enter/:cmd", s.enter)
	return s
}

func (s *ContainerServer) enter(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	cmd, err := hex.DecodeString(p[0].Value)
	if err != nil {
		roundtrip.ReplyJSON(w, http.StatusInternalServerError, trace.Wrap(err).Error())
		return
	}

	log.Printf("entering command: %v", cmd)
	ws := &enterHandler{
		cmd: string(cmd),
		c:   s.c,
	}
	defer ws.Close()
	ws.Handler().ServeHTTP(w, r)
}

type enterHandler struct {
	cmd string
	c   libcontainer.Container
}

func (w *enterHandler) Close() error {
	log.Printf("enterHandler.Close()")
	return nil
}

func (w *enterHandler) handle(ws *websocket.Conn) {
	defer ws.Close()
	err := w.enter(ws)
	if err != nil {
		log.Printf("enter error: %v", err)
	}
}

func (w *enterHandler) enter(ws *websocket.Conn) error {
	log.Printf("enter command: %v", w.cmd)

	defer ws.Close()
	return startProcess(w.c, []string{w.cmd}, ws)
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
