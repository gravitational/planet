package agent

import (
	"io"
	"net"
	"time"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	serfClient "github.com/gravitational/planet/Godeps/_workspace/src/github.com/hashicorp/serf/client"
	"github.com/gravitational/planet/lib/agent/cache"
	"github.com/gravitational/planet/lib/agent/health"
	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
)

type Agent interface {
	Start() error
	Close() error
	Join(peers []string) error

	health.CheckerRepository
}

type Config struct {
	// Name of the agent - hostname if not provided
	Name string

	// Address for serf layer traffic
	BindAddr string

	// RPC address for local agent communication
	RPCAddr string

	// RPC address of local serf node
	SerfRPCAddr string

	// Peers lists the nodes that are part of the initial serf cluster configuration
	Peers []string

	// Set of tags for the agent.
	// Tags is a trivial means for adding extra semantic information.
	Tags map[string]string

	// Cache used by the agent to persist health stats.
	Cache cache.Cache

	LogOutput io.Writer
}

func New(config *Config) (Agent, error) {
	clientConfig := &serfClient.Config{
		Addr: config.SerfRPCAddr,
	}
	client, err := serfClient.ClientFromConfig(clientConfig)
	if err != nil {
		return nil, trace.Wrap(err, "failed to connect to serf")
	}
	listener, err := net.Listen("tcp", config.RPCAddr)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	agent := &agent{
		serfClient: client,
		name:       config.Name,
		logOutput:  config.LogOutput,
		cache:      config.Cache,
	}
	agent.rpc = newRPCServer(agent, listener)
	return agent, nil
}

type agent struct {
	health.Checkers

	serfClient *serfClient.RPCClient

	// RPC server used by agent for client communication as well as
	// status sync with other agents.
	rpc *server
	// cache persists node status history.
	cache cache.Cache

	// Chan used to stream serf events.
	eventc chan map[string]interface{}

	name      string
	logOutput io.Writer
}

var _ Agent = (*agent)(nil)

func (r *agent) Start() error {
	var allEvents string
	eventc := make(chan map[string]interface{})
	_, err := r.serfClient.Stream(allEvents, eventc)
	if err != nil {
		return trace.Wrap(err, "failed to stream events from serf")
	}
	r.eventc = eventc

	go r.statusUpdateLoop()
	go r.serfEventLoop(allEvents)
	return nil
}

func (r *agent) Join(peers []string) error {
	noReplay := false
	numJoined, err := r.serfClient.Join(peers, noReplay)
	if err != nil {
		return trace.Wrap(err)
	}
	log.Infof("joined %d nodes", numJoined)
	return nil
}

func (r *agent) Close() (err error) {
	// FIXME: shutdown RPC server
	err = r.serfClient.Close()
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

func (r *agent) getStatus() (status *pb.NodeStatus, err error) {
	return r.runChecks(), nil
}

func (r *agent) runChecks() *pb.NodeStatus {
	reporter := health.NewDefaultReporter(r.name)
	for _, c := range r.Checkers {
		log.Infof("running checker %s", c.Name())
		c.Check(reporter)
	}
	return reporter.Status()
}

func (r *agent) statusUpdateLoop() {
	const updateTimeout = 1 * time.Minute
	for {
		tick := time.After(updateTimeout)
		select {
		case <-tick:
			status := r.runChecks()
			err := r.cache.UpdateNode(status)
			if err != nil {
				log.Errorf("error updating node status: %v", err)
			}
		}
	}
}

func (r *agent) serfEventLoop(filter string) {

	for {
		select {
		case resp := <-r.eventc:
			log.Infof("serf event: %v (%T)", resp, resp)
			// case <-ctx.Done():
			// 	r.serfClient.Stop(handle)
			// 	return
		}
	}
}
