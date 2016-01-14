package agent

import (
	"errors"
	"time"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/hashicorp/serf/serf"
	"github.com/gravitational/trace"
)

type queryCommand string

const (
	cmdStatus queryCommand = "status"
)

var errUnknownQuery = errors.New("unknown query")

type agentQuery struct {
	*serf.Serf
	resp      *serf.QueryResponse
	cmd       string
	timeout   time.Duration
	responses map[string][]byte
}

func (r *agentQuery) start() (err error) {
	conf := &serf.QueryParam{
		Timeout: r.timeout,
	}
	var noPayload []byte
	r.resp, err = r.Serf.Query(r.cmd, noPayload, conf)
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

func (r *agentQuery) run() error {
	if err := r.start(); err != nil {
		return trace.Wrap(err, "failed to start serf query")
	}
	r.responses = make(map[string][]byte)
	for response := range r.resp.ResponseCh() {
		log.Infof("response from %s: %s", response.From, response)
		r.responses[response.From] = response.Payload
	}
	return nil
}
