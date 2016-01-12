package agent

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	serfAgent "github.com/gravitational/planet/Godeps/_workspace/src/github.com/hashicorp/serf/command/agent"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/hashicorp/serf/serf"
	"github.com/gravitational/planet/lib/agent/cache"
	"github.com/gravitational/planet/lib/agent/health"
	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
)

type Agent interface {
	Start() error
	Close() error
	ShutdownCh() <-chan struct{}
	Join([]string, bool) (int, error)

	health.CheckerRepository
}

type Config struct {
	// Name of the agent - hostname if not provided
	Name string

	// Address for serf layer traffic
	BindAddr string

	// RPC address for local agent communication
	RPCAddr string

	// Set of tags for the agent.
	// Tags is a trivial means for adding extra semantic information.
	Tags map[string]string

	// Cache used by the agent to persist health stats.
	Cache cache.Cache

	LogOutput io.Writer
}

func New(config *Config) (Agent, error) {
	agentConfig := setupAgent(config)
	serfConfig, err := setupSerf(config)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	serfAgent, err := serfAgent.Create(agentConfig, serfConfig, config.LogOutput)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	agent := &agent{
		Agent:     serfAgent,
		name:      config.Name,
		rpcAddr:   config.RPCAddr,
		logOutput: config.LogOutput,
		cache:     config.Cache,
	}
	serfAgent.RegisterEventHandler(agent)
	return agent, nil
}

type agent struct {
	*serfAgent.Agent
	health.Checkers

	rpcAddr string
	rpc     *server

	cache cache.Cache

	name      string
	logOutput io.Writer
}

func (r *agent) Start() error {
	err := r.Agent.Start()
	if err != nil {
		return trace.Wrap(err)
	}
	listener, err := net.Listen("tcp", r.rpcAddr)
	if err != nil {
		return trace.Wrap(err)
	}
	r.rpc = newRPCServer(r, listener)
	go r.serviceLoop()
	return nil
}

// agent.EventHandler
func (r *agent) HandleEvent(event serf.Event) {
	if query, ok := event.(*serf.Query); ok {
		switch query.Name {
		case string(cmdStatus):
			if err := r.handleStatus(query); err != nil {
				log.Errorf("failed to handle status query: %v", err)
			}
		default:
			if err := query.Respond([]byte(errUnknownQuery.Error())); err != nil {
				log.Errorf("failed to respond to query: %v", err)
			}
		}
	}
}

func (r *agent) Close() error {
	// FIXME: shutdown RPC server
	errLeave := r.Leave()
	errShutdown := r.Shutdown()
	if errShutdown != nil {
		return errShutdown
	}
	return errLeave
}

func (r *agent) handleStatus(q *serf.Query) error {
	status := r.runChecks()
	payload, err := json.Marshal(status)
	if err != nil {
		return trace.Wrap(err)
	}
	if err := q.Respond(payload); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

func (r *agent) runChecks() *pb.NodeStatus {
	reporter := health.NewDefaultReporter(r.name)
	for _, c := range r.Checkers {
		log.Infof("running checker %s", c.Name())
		c.Check(reporter)
	}
	return reporter.Status()
}

func (r *agent) serviceLoop() {
	const serviceTimeout = 1 * time.Minute
	for {
		tick := time.After(serviceTimeout)
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

type queryCommand string

const (
	cmdStatus queryCommand = "status"
)

var errUnknownQuery = errors.New("unknown query")

func setupAgent(config *Config) *serfAgent.Config {
	c := serfAgent.DefaultConfig()
	c.BindAddr = config.BindAddr
	return c
}

func setupSerf(config *Config) (*serf.Config, error) {
	host, port, err := net.SplitHostPort(config.BindAddr)
	if err != nil {
		return nil, err
	}
	c := serf.DefaultConfig()
	c.Init()
	c.MemberlistConfig.BindAddr = host
	c.MemberlistConfig.BindPort = mustAtoi(port)
	for key, value := range config.Tags {
		c.Tags[key] = value
	}
	return c, nil
}

func mustAtoi(value string) int {
	result, err := strconv.Atoi(value)
	if err != nil {
		panic(err)
	}
	return result
}

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
