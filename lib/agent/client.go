package agent

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	serf "github.com/gravitational/planet/Godeps/_workspace/src/github.com/hashicorp/serf/client"
	"github.com/gravitational/planet/lib/agent/monitoring"
)

// Client defines an obligation to handle status queries.
// Client is used to communicate to the cluster of test agents.
type Client interface {
	Status() (*monitoring.Status, error)
}

type client struct {
	*serf.RPCClient
}

// NewClient creates a new agent client.
func NewClient(rpcAddr string) (Client, error) {
	serfClient, err := serf.NewRPCClient(rpcAddr)
	if err != nil {
		return nil, err
	}
	return &client{
		RPCClient: serfClient,
	}, nil
}

// Status reports the status of the serf cluster.
func (r *client) Status() (*monitoring.Status, error) {
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

// memberNames returns a list of names of all currently active nodes.
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

// query represents a running agent query.
type query struct {
	responsec chan serf.NodeResponse
	responses map[string][]byte
	members   []string
}

// newQuery creates a new query for the specified query command and a list of known members.
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

// run runs a query until all responses have been collected or a timeout is signalled.
func (r *query) run() error {
	var responsesFrom []string
	for response := range r.responsec {
		log.Infof("response from %s: %s", response.From, response)
		r.responses[response.From] = response.Payload
		responsesFrom = append(responsesFrom, response.From)
		// FIXME: bail out as soon as responses from all known nodes have been collected
		// if len(responsesFrom) == len(r.members) && sliceEquals(responsesFrom, r.members) {
		// 	r.responsec = nil
		// }
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
