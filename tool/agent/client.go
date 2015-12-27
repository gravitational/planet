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
}

func newClient(rpcAddr string) (*client, error) {
	serfClient, err := serf.NewRPCClient(rpcAddr)
	if err != nil {
		return nil, err
	}
	return &client{
		RPCClient: serfClient,
	}, nil
}

func (r *client) status() (*monitoring.Status, error) {
	memberNodes, err := r.memberNames()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	q, err := newQuery(cmdStatus, r, memberNodes)
	if err != nil {
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
	if !sliceEquals(healthyNodes, memberNodes) {
		status.SystemStatus = monitoring.SystemStatusDegraded
	} else {
		status.SystemStatus = monitoring.SystemStatusRunning
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
	responsec chan serf.NodeResponse
	responses map[string][]byte
	members   []string
}

func newQuery(cmd queryCommand, client *client, members []string) (result *query, err error) {
	responsec := make(chan serf.NodeResponse, 1)
	params := &serf.QueryParam{
		Name:    string(cmd),
		Timeout: 1 * time.Second,
		RespCh:  responsec,
	}
	result = &query{
		responsec: responsec,
		responses: make(map[string][]byte),
		members:   members,
	}
	err = client.Query(params)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return result, nil
}

func (r *query) run() error {
	var responsesFrom []string
	for r.responsec != nil {
		select {
		case response, ok := <-r.responsec:
			log.Infof("response from %s: %s", response.From, response)
			if !ok {
				r.responsec = nil
			} else {
				r.responses[response.From] = response.Payload
				responsesFrom = append(responsesFrom, response.From)
				if len(responsesFrom) == len(r.members) && sliceEquals(responsesFrom, r.members) {
					r.responsec = nil
				}
			}
		}
	}
	return nil
}

// sliceEquals returns true if a equals b.
// Side-effect: the slice arguments are sorted in-place.
func sliceEquals(a, b []string) bool {
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
