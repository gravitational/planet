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
	serf "github.com/gravitational/planet/Godeps/_workspace/src/github.com/hashicorp/serf/serf"
	"github.com/gravitational/planet/tool/agent/monitoring"
	"github.com/gravitational/planet/tool/agent/monitoring/check"
)

var errUnknownQuery = errors.New("unknown query")

type testAgent struct {
	*serfAgent.Agent
	config    *config
	logOutput io.Writer
}

type config struct {
	bindAddr     string
	rpcAddr      string
	kubeHostPort string
	mode         agentMode
	tags         map[string]string
}

type reporter struct {
	status *monitoring.Status
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

type checker struct {
	tags    map[string]string
	checker check.Checker
}

var checkers []checker

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
	c.Tags["mode"] = string(config.mode)
	return c
}

func setupSerf(config *config) (*serf.Config, error) {
	host, port, err := net.SplitHostPort(config.bindAddr)
	if err != nil {
		return nil, err
	}
	c := serf.DefaultConfig()
	c.MemberlistConfig.BindAddr = host
	c.MemberlistConfig.BindPort = mustAtoi(port)
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
	var reporter *reporter
	ctx := &check.Context{
		Reporter: reporter,
		Config: &check.Config{
			KubeHostPort: r.config.kubeHostPort,
		},
	}
	for _, t := range check.Testers {
		t.Checker.Check(ctx)
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

func (r *reporter) Add(name string, payload string) {
	// TODO
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

func AddChecker(c check.Checker, tags map[string]string) {
	checkers = append(checkers, checker{checker: c, tags: tags})
}
