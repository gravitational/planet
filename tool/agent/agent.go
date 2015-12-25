package main

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
	systemMonitoring "github.com/gravitational/planet/lib/monitoring"
	"github.com/gravitational/planet/tool/agent/monitoring"
	"github.com/gravitational/planet/tool/agent/monitoring/health"
)

var errUnknownQuery = errors.New("unknown query")

type testAgent struct {
	*serfAgent.Agent
	config    *config
	logOutput io.Writer
}

type config struct {
	name         string
	bindAddr     string
	rpcAddr      string
	kubeHostPort string
	mode         agentMode
	tags         map[string]string
}

type reporter struct {
	status *monitoring.NodeStatus
}

type agentMode string

const (
	master agentMode = "master"
	node             = "node"
)

type queryCommand string

const (
	cmdStatus queryCommand = "status"
)

func newAgent(config *config, logOutput io.Writer) (*testAgent, error) {
	agentConfig := setupAgent(config)
	serfConfig, err := setupSerf(config)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	agent, err := serfAgent.Create(agentConfig, serfConfig, logOutput)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	testAgent := &testAgent{
		Agent:     agent,
		config:    config,
		logOutput: logOutput,
	}
	agent.RegisterEventHandler(testAgent)
	return testAgent, nil
}

func setupAgent(config *config) *serfAgent.Config {
	c := serfAgent.DefaultConfig()
	c.BindAddr = config.bindAddr
	c.RPCAddr = config.rpcAddr
	return c
}

func setupSerf(config *config) (*serf.Config, error) {
	host, port, err := net.SplitHostPort(config.bindAddr)
	if err != nil {
		return nil, err
	}
	c := serf.DefaultConfig()
	c.Init()
	c.MemberlistConfig.BindAddr = host
	c.MemberlistConfig.BindPort = mustAtoi(port)
	c.Tags["mode"] = string(config.mode)
	return c, nil
}

// FIXME: hide AgentIPC behind a meaningful interface
func (r *testAgent) start() (*serfAgent.AgentIPC, error) {
	err := r.Start()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	listener, err := net.Listen("tcp", r.config.rpcAddr)
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
	log.Infof("testAgent:handleStatus for %v", q)
	reporter := &reporter{status: &monitoring.NodeStatus{Name: r.config.name}}
	config := &health.Config{
		KubeHostPort: r.config.kubeHostPort,
	}
	log.Infof("available checkers: %#v, node tags: %#v", health.Testers, r.SerfConfig().Tags)
	for _, t := range health.Testers {
		if tagsInclude(r.SerfConfig().Tags, t.Tags) {
			log.Infof("running checker %s", t.Name)
			t.Check(reporter, config)
		}
	}
	payload, err := reporter.encode()
	if err != nil {
		return trace.Wrap(err)
	}
	if err := q.Respond(payload); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

func (r *reporter) Add(err error) {
	// unused
}

func (r *reporter) AddNamed(name string, err error) {
	r.status.SystemStatus.Services = append(r.status.SystemStatus.Services, systemMonitoring.ServiceStatus{
		Name:    name,
		Message: err.Error(),
		Status:  systemMonitoring.StatusFailed,
	})
}

func (r *reporter) encode() ([]byte, error) {
	return json.Marshal(r.status)
}

func mustAtoi(value string) int {
	result, err := strconv.Atoi(value)
	if err != nil {
		panic(err)
	}
	return result
}

// tagsInclude determines if any items from include are included in source.
func tagsInclude(source map[string]string, include health.Tags) bool {
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
