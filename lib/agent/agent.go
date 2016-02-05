package agent

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/gravitational/log"
	"github.com/gravitational/planet/lib/agent/cache"
	"github.com/gravitational/planet/lib/agent/health"
	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
	"github.com/gravitational/trace"
	serf "github.com/hashicorp/serf/client"
	"github.com/jonboulle/clockwork"
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
		clock:      clockwork.NewRealClock(),
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
	rpc RPCServer
	// cache persists node status history.
	cache cache.Cache

	// dialRPC is a factory function to create clients to other agents.
	// If future, agent address discovery will happen through serf.
	dialRPC dialRPC

	// done is a channel used for cleanup.
	done chan struct{}
	// eventc is a channel used to stream serf events.
	eventc chan map[string]interface{}

	// clock abstracts away access to the time package to allow
	// testing.
	clock clockwork.Clock
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

// runChecks executes the monitoring tests configured for this agent.
func (r *agent) runChecks(ctx context.Context) *pb.NodeStatus {
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

// statusUpdateTimeout is the amount of time to wait between status update collections.
const statusUpdateTimeout = 30 * time.Second

// statusQueryWaitTimeout is the amount of time to wait for status query reply.
const statusQueryWaitTimeout = 10 * time.Second

// statusUpdateLoop is a long running background process that periodically
// updates the health status of the cluster by querying status of other active
// cluster members.
func (r *agent) statusUpdateLoop() {
	for {
		select {
		case <-r.clock.After(statusUpdateTimeout):
			ctx, cancel := context.WithTimeout(context.Background(), statusQueryWaitTimeout)
			go func() {
				defer cancel() // close context if collection finishes before the deadline
				status, err := r.collectStatus(ctx)
				if err != nil {
					log.Infof("error collecting system status: %v", err)
					return
				}
				if err = r.cache.Update(status); err != nil {
					log.Infof("error updating system status in cache: %v", err)
				}
			}()
			select {
			case <-ctx.Done():
				if ctx.Err() == context.DeadlineExceeded {
					log.Infof("timed out collecting system status")
				}
			case <-r.done:
				cancel()
				return
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

// collectStatus obtains the cluster status by querying statuses of
// known cluster members.
func (r *agent) collectStatus(ctx context.Context) (systemStatus *pb.SystemStatus, err error) {
	systemStatus = &pb.SystemStatus{Status: pb.SystemStatus_Unknown}

	members, err := r.serfClient.Members()
	if err != nil {
		return nil, trace.Wrap(err, "failed to query serf members")
	}

	statuses := make(chan *statusResponse, len(members))
	var wg sync.WaitGroup

	wg.Add(len(members))
	for _, member := range members {
		if r.name == member.Name {
			go r.getLocalStatus(ctx, member, statuses, &wg)
		} else {
			go r.getStatusFrom(ctx, member, statuses, &wg)
		}
	}
	wg.Wait()
	close(statuses)

	for status := range statuses {
		nodeStatus := status.NodeStatus
		if status.err != nil {
			log.Infof("failed to query node %s(%v) status: %v", status.member.Name, status.member.Addr, status.err)
			nodeStatus = unknownNodeStatus(&status.member)
		}
		systemStatus.Nodes = append(systemStatus.Nodes, nodeStatus)
	}

	return systemStatus, nil
}

// collectLocalStatus executes monitoring tests on the local node.
func (r *agent) collectLocalStatus(ctx context.Context, local *serf.Member) (status *pb.NodeStatus, err error) {
	status = r.runChecks(ctx)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	status.MemberStatus = statusFromMember(local)
	if err = r.cache.UpdateNode(status); err != nil {
		log.Infof("failed to update node in cache: %v", err)
	}

	return status, nil
}

// getLocalStatus obtains local node status in background.
func (r *agent) getLocalStatus(ctx context.Context, local serf.Member, respc chan<- *statusResponse, wg *sync.WaitGroup) {
	defer wg.Done()
	status, err := r.collectLocalStatus(ctx, &local)
	resp := &statusResponse{member: local}
	if err != nil {
		resp.err = trace.Wrap(err)
	} else {
		resp.NodeStatus = status
	}
	select {
	case respc <- resp:
	case <-r.done:
	}
}

// getStatusFrom obtains node status from the node identified by member in background.
func (r *agent) getStatusFrom(ctx context.Context, member serf.Member, respc chan<- *statusResponse, wg *sync.WaitGroup) {
	defer wg.Done()
	client, err := r.dialRPC(&member)
	resp := &statusResponse{member: member}
	if err != nil {
		resp.err = trace.Wrap(err)
	} else {
		defer client.Close()
		var status *pb.NodeStatus
		status, err = client.LocalStatus(ctx)
		if err != nil {
			resp.err = trace.Wrap(err)
		} else {
			resp.NodeStatus = status
		}
	}
	select {
	case respc <- resp:
	case <-r.done:
	}
}

// statusResponse describes a status response from a background process that obtains
// health status on the specified serf node.
type statusResponse struct {
	*pb.NodeStatus
	member serf.Member
	err    error
}

// recentStatus returns the last known cluster status.
func (r *agent) recentStatus() (*pb.SystemStatus, error) {
	return r.cache.RecentStatus()
}

// recentLocalStatus returns the last known local node status.
func (r *agent) recentLocalStatus() (*pb.NodeStatus, error) {
	return r.cache.RecentNodeStatus(r.name)
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

// unknownNodeStatus returns a new node status initialized with `unknown`.
func unknownNodeStatus(member *serf.Member) *pb.NodeStatus {
	return &pb.NodeStatus{
		Name:         member.Name,
		Status:       pb.NodeStatus_Unknown,
		MemberStatus: statusFromMember(member),
	}
}

// statusFromMember returns new member status value for the specified serf member.
func statusFromMember(member *serf.Member) *pb.MemberStatus {
	return &pb.MemberStatus{
		Name:   member.Name,
		Status: toMemberStatus(member.Status),
		Tags:   member.Tags,
		Addr:   fmt.Sprintf("%s:%d", member.Addr.String(), member.Port),
	}
}
