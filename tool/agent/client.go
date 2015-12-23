package main

import (
	"time"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	serf "github.com/gravitational/planet/Godeps/_workspace/src/github.com/hashicorp/serf/client"
	"github.com/gravitational/planet/tool/agent/monitoring"
)

type client struct {
	*serf.RPCClient
	node string
}

type query struct {
	ackc      chan string
	responsec chan serf.NodeResponse
	params    *serf.QueryParam
	acks      map[string]struct{}
	response  map[string][]byte
}

func newClient(node, rpcAddr string) (*client, error) {
	serfClient, err := serf.NewRPCClient(rpcAddr)
	if err != nil {
		return nil, err
	}
	return &client{
		RPCClient: serfClient,
		node:      node,
	}, nil
}

func (r *client) status() (*monitoring.Status, error) {
	// members, err := r.Members()
	// if err != nil {
	// 	return nil, trace.Wrap(err)
	// }
	q := newQuery(cmdStatus)
	if err := r.Query(q.params); err != nil {
		return nil, trace.Wrap(err)
	}
	if err := q.run(); err != nil {
		return nil, trace.Wrap(err)
	}
	var status monitoring.Status
	// TODO: interpret query results as monitoring.Status
	return &status, nil
}

func newQuery(cmd queryCommand) *query {
	ackc := make(chan string, 1)
	responsec := make(chan serf.NodeResponse, 1)
	params := &serf.QueryParam{
		Name:       string(cmd),
		Timeout:    200 * time.Millisecond,
		RequestAck: true,
		AckCh:      ackc,
		RespCh:     responsec,
	}
	return &query{
		ackc:      ackc,
		responsec: responsec,
		params:    params,
		acks:      make(map[string]struct{}),
		response:  make(map[string][]byte),
	}
}

func (r *query) run() error {
	for {
		select {
		case ack, ok := <-r.ackc:
			log.Infof("ack: %s", ack)
			if !ok {
				r.ackc = nil
			} else {
				r.acks[ack] = struct{}{}
			}
		case response, ok := <-r.responsec:
			log.Infof("response: %s (%v -> %T)", response, ok, response)
			if !ok {
				r.responsec = nil
			} else {
				r.response[response.From] = response.Payload
			}
		}
		if r.ackc == nil && r.responsec == nil {
			return nil
		}
	}
}
