package agent

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"strconv"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	serfAgent "github.com/gravitational/planet/Godeps/_workspace/src/github.com/hashicorp/serf/command/agent"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/hashicorp/serf/serf"
	"github.com/gravitational/planet/lib/agent/monitoring"
)

type Agent interface {
	Start() (Closer, error)
	Leave() error
	Shutdown() error
	ShutdownCh() <-chan struct{}
	Join([]string, bool) (int, error)
}

type Closer interface {
	Shutdown()
}

var errUnknownQuery = errors.New("unknown query")

type testAgent struct {
	*serfAgent.Agent
	config    *Config
	logOutput io.Writer
}

type Config struct {
	Name     string
	BindAddr string
	RPCAddr  string
	Mode     Mode
	Tags     map[string]string
}

type Mode string

const (
	Master Mode = "master"
	Node        = "node"
)

type queryCommand string

const (
	cmdStatus queryCommand = "status"
)

func NewAgent(config *Config, logOutput io.Writer) (Agent, error) {
	agentConfig := setupAgent(config)
	serfConfig, err := setupSerf(config)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	serfAgent, err := serfAgent.Create(agentConfig, serfConfig, logOutput)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	testAgent := &testAgent{
		Agent:     serfAgent,
		config:    config,
		logOutput: logOutput,
	}
	serfAgent.RegisterEventHandler(testAgent)
	return testAgent, nil
}

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
	c.Tags["mode"] = string(config.Mode)
	return c, nil
}

func (r *testAgent) Start() (Closer, error) {
	err := r.Agent.Start()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	listener, err := net.Listen("tcp", r.config.RPCAddr)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	authKey := ""
	return serfAgent.NewAgentIPC(r.Agent, authKey, listener, r.logOutput, nil), nil
}

// agent.EventHandler
func (r *testAgent) HandleEvent(event serf.Event) {
	log.Infof("event: %#v", event)
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

func (r *testAgent) handleStatus(q *serf.Query) error {
	reporter := monitoring.NewDefaultReporter(r.config.Name)
	log.Infof("available checkers: %v, node tags: %#v", monitoring.Testers, r.SerfConfig().Tags)
	for _, t := range monitoring.Testers {
		if tagsInclude(r.SerfConfig().Tags, t.Tags) {
			log.Infof("running checker %s", t.Name)
			t.Check(reporter)
		}
	}
	payload, err := json.Marshal(reporter.Status())
	if err != nil {
		return trace.Wrap(err)
	}
	if err := q.Respond(payload); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

func mustAtoi(value string) int {
	result, err := strconv.Atoi(value)
	if err != nil {
		panic(err)
	}
	return result
}

// tagsInclude determines if any items from include are included in source.
func tagsInclude(source map[string]string, include monitoring.Tags) bool {
	for key, values := range include {
		if sourceValue, ok := source[key]; ok && inSlice(sourceValue, values) {
			return true
		}
	}
	return false
}

// inSlice determines if value is in slice.
func inSlice(value string, slice []string) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}
