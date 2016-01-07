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
	"github.com/gravitational/planet/lib/agent/health"
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
	RPCAddr  string
	Tags     map[string]string
}

func New(config *Config, logOutput io.Writer) (Agent, error) {
	agentConfig := setupAgent(config)
	serfConfig, err := setupSerf(config)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	serfAgent, err := serfAgent.Create(agentConfig, serfConfig, logOutput)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	agent := &agent{
		Agent:     serfAgent,
		config:    config,
		logOutput: logOutput,
	}
	serfAgent.RegisterEventHandler(agent)
	return agent, nil
}

type agent struct {
	*serfAgent.Agent
	health.Checkers

	ipc *serfAgent.AgentIPC

	config    *Config
	logOutput io.Writer
}

func (r *agent) Start() error {
	err := r.Agent.Start()
	if err != nil {
		return trace.Wrap(err)
	}
	listener, err := net.Listen("tcp", r.config.RPCAddr)
	if err != nil {
		return trace.Wrap(err)
	}
	authKey := ""
	r.ipc = serfAgent.NewAgentIPC(r.Agent, authKey, listener, r.logOutput, nil)
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
	if r.ipc != nil {
		r.ipc.Shutdown()
		r.ipc = nil
	}
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

func (r *agent) runChecks() *health.NodeStatus {
	reporter := health.NewDefaultReporter(r.config.Name)
	for _, c := range r.Checkers {
		log.Infof("running checker %s", c.Name())
		c.Check(reporter)
	}
	return reporter.Status()
}

type queryCommand string

const (
	cmdStatus queryCommand = "status"
)

var errUnknownQuery = errors.New("unknown query")

func setupAgent(config *Config) *serfAgent.Config {
	c := serfAgent.DefaultConfig()
	c.BindAddr = config.BindAddr
	c.RPCAddr = config.RPCAddr
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

// queryRunner
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
		return err
	}
	r.responses = make(map[string][]byte)
	for response := range r.resp.ResponseCh() {
		log.Infof("response from %s: %s", response.From, response)
		r.responses[response.From] = response.Payload
	}
	return nil
}
