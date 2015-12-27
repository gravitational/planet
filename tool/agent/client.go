package main

import (
	"encoding/json"
	"sort"
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
	memberNodes, err := r.memberNames()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	q := newQuery(cmdStatus)
	if err = r.Query(q.params); err != nil {
		return nil, trace.Wrap(err)
	}
	if err = q.run(); err != nil {
		return nil, trace.Wrap(err)
	}
	var status monitoring.Status
	var healthyNodes []string
	for node, response := range q.responses {
		var nodeStatus monitoring.NodeStatus
		if err = json.Unmarshal(response, &nodeStatus); err != nil {
			return nil, trace.Wrap(err, "failed to unmarshal query result")
		}
		status.Nodes = append(status.Nodes, nodeStatus)
		if len(nodeStatus.Events) == 0 {
			healthyNodes = append(healthyNodes, node)
		}
	}
	if !slicesEqual(healthyNodes, memberNodes) {
		status.Status = monitoring.StatusDegraded
	} else {
		status.Status = monitoring.StatusRunning
	}
	return &status, nil
}

func (r *client) memberNames() ([]string, error) {
	members, err := r.Members()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	var nodes []string
	for _, member := range members {
		nodes = append(nodes, member.Name)
	}
	return nodes, nil
}

type query struct {
	ackc      chan string
	responsec chan serf.NodeResponse
	params    *serf.QueryParam
	acks      map[string]struct{}
	responses map[string][]byte
}

func newQuery(cmd queryCommand) *query {
	ackc := make(chan string, 1)
	responsec := make(chan serf.NodeResponse, 1)
	params := &serf.QueryParam{
		Name:       string(cmd),
		Timeout:    1 * time.Second,
		RequestAck: true,
		AckCh:      ackc,
		RespCh:     responsec,
	}
	return &query{
		ackc:      ackc,
		responsec: responsec,
		params:    params,
		acks:      make(map[string]struct{}),
		responses: make(map[string][]byte),
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
				r.responses[response.From] = response.Payload
			}
		}
		if r.ackc == nil && r.responsec == nil {
			return nil
		}
	}
}

// slicesEqual returns true if a equals b.
// Side-effect: the slice arguments are sorted in-place.
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sort.Sort(sort.StringSlice(a))
	sort.Sort(sort.StringSlice(b))
	for i, _ := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
