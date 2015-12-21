package main

import (
	"os"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	serfAgent "github.com/gravitational/planet/Godeps/_workspace/src/github.com/hashicorp/serf/command/agent"
	serf "github.com/gravitational/planet/Godeps/_workspace/src/github.com/hashicorp/serf/serf"
)

type testAgent struct {
	*serfAgent.Agent
}

type config struct {
	bindAddr string
	rpcAddr  string
	mode     mode
}

type mode string

const (
	master mode = "master"
	node        = "node"
)

func newAgent(config *config) (*testAgent, error) {
	logOutput := os.Stderr
	agentConfig := serfAgent.DefaultConfig()
	agentConfig.BindAddr = config.bindAddr
	agentConfig.RPCAddr = config.rpcAddr
	agentConfig.Tags["mode"] = string(config.mode)
	agent, err := serfAgent.Create(agentConfig, serf.DefaultConfig(), logOutput)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	testAgent := &testAgent{
		Agent: agent,
	}
	agent.RegisterEventHandler(testAgent)
	return testAgent, nil
}

// agent.EventHandler
func (r *testAgent) HandleEvent(event serf.Event) {
	log.Infof("event: %#v", event)
}
