package agent

import (
	"encoding/json"
	"time"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	serf "github.com/gravitational/planet/Godeps/_workspace/src/github.com/hashicorp/serf/client"
	"github.com/gravitational/planet/lib/agent/health"
	"github.com/gravitational/planet/lib/util"
)

// Client is an interface to communicate with the serf cluster.
type Client interface {
	Status() (*health.Status, error)
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
func (r *client) Status() (*health.Status, error) {
	memberNodes, err := r.memberNames()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	q := &clientQuery{
		client: r,
		cmd:    string(cmdStatus),
	}
	if err = q.run(); err != nil {
		return nil, trace.Wrap(err)
	}
	var status health.Status
	var healthyNodes []string
	for node, response := range q.responses {
		var nodeStatus health.NodeStatus
		if err = json.Unmarshal(response, &nodeStatus); err != nil {
			return nil, trace.Wrap(err, "failed to unmarshal query result")
		}
		status.Nodes = append(status.Nodes, nodeStatus)
		if len(nodeStatus.Probes) == 0 {
			healthyNodes = append(healthyNodes, node)
		}
	}
	if !util.StringSliceEquals(healthyNodes, memberNodes) {
		status.SystemStatus = health.SystemStatusDegraded
	} else {
		status.SystemStatus = health.SystemStatusRunning
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

// queryRunner
type clientQuery struct {
	client    *client
	responsec chan serf.NodeResponse
	cmd       string
	timeout   time.Duration
	responses map[string][]byte
}

func (r *clientQuery) start() error {
	r.responsec = make(chan serf.NodeResponse, 1)
	conf := &serf.QueryParam{
		Name:    r.cmd,
		Timeout: r.timeout,
		RespCh:  r.responsec,
	}
	err := r.client.Query(conf)
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

func (r *clientQuery) run() error {
	if err := r.start(); err != nil {
		return err
	}
	r.responses = make(map[string][]byte)
	for response := range r.responsec {
		log.Infof("response from %s: %s", response.From, response)
		r.responses[response.From] = response.Payload
	}
	return nil
}
