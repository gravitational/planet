package agent

import (
	"fmt"
	"net"
	"time"

	"github.com/gravitational/log"
	"github.com/gravitational/planet/lib/agent/cache"
	"github.com/gravitational/planet/lib/agent/health"
	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
	"github.com/gravitational/trace"
	serf "github.com/hashicorp/serf/client"
	"golang.org/x/net/context"
)

// Agent is the interface to interact with the planet agent.
type Agent interface {
	// Start starts agent's background jobs.
	Start() error
	// Close stops background activity and releases resources.
	Close() error
	// Join makes an attempt to join a cluster specified by the list of peers.
	Join(peers []string) error
	// LocalStatus reports the health status of the local agent node.
	LocalStatus(context.Context) (*pb.NodeStatus, error)

	health.CheckerRepository
}

type Config struct {
	// Name of the agent unique within the cluster.
	// Names are used as a unique id within a serf cluster, so
	// it is important to avoid clashes.
	//
	// Name must match the name of the local serf agent so that the agent
	// can match itself to a serf member.
	Name string

	// RPCAddrs is a list of addresses agent binds to for RPC traffic.
	//
	// Usually, at least two address are used for operation.
	// Localhost is a convenience for local communication.  Cluster-visible
	// IP is required for proper inter-communication between agents.
	RPCAddrs []string

	// RPC address of local serf node.
	SerfRPCAddr string

	// Peers lists the nodes that are part of the initial serf cluster configuration.
	// This is not a final cluster configuration and new nodes or node updates
	// are still possible.
	Peers []string

	// Set of tags for the agent.
	// Tags is a trivial means for adding extra semantic information to an agent / node.
	Tags map[string]string

	// Cache is a short-lived storage used by the agent to persist latest health stats.
	Cache cache.Cache
}

// New creates an instance of an agent based on configuration options given in config.
func New(config *Config) (Agent, error) {
	clientConfig := &serf.Config{
		Addr: config.SerfRPCAddr,
	}
	client, err := serf.ClientFromConfig(clientConfig)
	if err != nil {
		return nil, trace.Wrap(err, "failed to connect to serf")
	}
	err = client.UpdateTags(config.Tags, nil)
	if err != nil {
		return nil, trace.Wrap(err, "failed to update serf agent tags")
	}
	var listeners []net.Listener
	defer func() {
		if err != nil {
			for _, listener := range listeners {
				listener.Close()
			}
		}
	}()
	for _, addr := range config.RPCAddrs {
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		listeners = append(listeners, listener)
	}
	agent := &agent{
		serfClient: client,
		name:       config.Name,
		cache:      config.Cache,
		dialRPC:    defaultDialRPC,
	}
	agent.rpc = newRPCServer(agent, listeners)
	return agent, nil
}

type agent struct {
	health.Checkers

	serfClient serfClient

	// Name of this agent.  Must be the same as the serf agent's name
	// running on the same node.
	name string
	// RPC server used by agent for client communication as well as
	// status sync with other agents.
	rpc *server
	// cache persists node status history.
	cache cache.Cache

	// dialRPC is a factory function to create clients to other agents.
	// If future, agent address discovery will happen through serf.
	dialRPC dialRPC

	// done is a channel used for cleanup.
	done chan struct{}
	// eventc is a channel used to stream serf events.
	eventc chan map[string]interface{}
}

// Start starts the agent's background tasks.
func (r *agent) Start() error {
	var allEvents string
	eventc := make(chan map[string]interface{})
	handle, err := r.serfClient.Stream(allEvents, eventc)
	if err != nil {
		return trace.Wrap(err, "failed to stream events from serf")
	}
	r.eventc = eventc
	r.done = make(chan struct{})

	go r.statusUpdateLoop()
	go r.serfEventLoop(allEvents, handle)
	return nil
}

// Join attempts to join a serf cluster identified by peers.
func (r *agent) Join(peers []string) error {
	noReplay := false
	numJoined, err := r.serfClient.Join(peers, noReplay)
	if err != nil {
		return trace.Wrap(err)
	}
	log.Infof("joined %d nodes", numJoined)
	return nil
}

// Close stops all background activity and releases the agent's resources.
func (r *agent) Close() (err error) {
	r.rpc.Stop()
	close(r.done)
	err = r.serfClient.Close()
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// LocalStatus reports the status of the local agent node.
func (r *agent) LocalStatus(ctx context.Context) (*pb.NodeStatus, error) {
	req := &pb.LocalStatusRequest{}
	resp, err := r.rpc.LocalStatus(ctx, req)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return resp.Status, nil
}

type dialRPC func(*serf.Member) (*client, error)

// TODO: pass context.Context here so that tests can also be cancelled
func (r *agent) getStatus(local *serf.Member) (status *pb.NodeStatus, err error) {
	status = r.runChecks()
	if local == nil {
		// FIXME: factor this out into a separate method (share the code with the server?)
		members, err := r.serfClient.Members()
		if err != nil {
			return nil, trace.Wrap(err, "failed to query status of local serf node")
		}
		for _, member := range members {
			if member.Name == r.name {
				local = &member
				break
			}
		}
		if local == nil {
			return nil, trace.Errorf("node is not part of serf cluster")
		}
	}
	status.MemberStatus = &pb.MemberStatus{
		Name:   local.Name,
		Status: toMemberStatus(local.Status),
		Tags:   local.Tags,
		Addr:   fmt.Sprintf("%s:%d", local.Addr.String(), local.Port),
	}
	return status, nil
}

func (r *agent) runChecks() *pb.NodeStatus {
	var reporter health.Probes
	// TODO: run tests in parallel
	for _, c := range r.Checkers {
		log.Infof("running checker %s", c.Name())
		c.Check(&reporter)
	}
	status := &pb.NodeStatus{
		Name:   r.name,
		Status: reporter.Status(),
		Probes: reporter.GetProbes(),
	}
	return status
}

func (r *agent) statusUpdateLoop() {
	const updateTimeout = 30 * time.Second
	for {
		select {
		case <-time.After(updateTimeout):
			status := r.runChecks()
			err := r.cache.UpdateNode(status)
			if err != nil {
				log.Errorf("error updating node status: %v", err)
			}
		case <-r.done:
			return
		}
	}
}

func (r *agent) serfEventLoop(filter string, handle serf.StreamHandle) {
	for {
		select {
		case resp := <-r.eventc:
			log.Infof("serf event: %v (%T)", resp, resp)
		case <-r.done:
			r.serfClient.Stop(handle)
			return
		}
	}
}

func toMemberStatus(status string) pb.MemberStatus_Type {
	switch MemberStatus(status) {
	case MemberAlive:
		return pb.MemberStatus_Alive
	case MemberLeaving:
		return pb.MemberStatus_Leaving
	case MemberLeft:
		return pb.MemberStatus_Left
	case MemberFailed:
		return pb.MemberStatus_Failed
	}
	return pb.MemberStatus_None
}
