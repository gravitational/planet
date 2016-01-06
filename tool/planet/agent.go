package main

import (
	"encoding/json"
	"os"
	"os/signal"
	"syscall"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/planet/lib/agent"
	"github.com/gravitational/planet/lib/agent/health"
	"github.com/gravitational/planet/lib/monitoring"
)

type agentRole monitoring.Role

func runAgent(conf *agent.Config, monitoringConf *monitoring.Config, join string) error {
	logOutput := os.Stderr
	if conf.Tags == nil {
		conf.Tags = make(map[string]string)
	}
	conf.Tags["role"] = string(monitoringConf.Role)
	agent, err := agent.New(conf, logOutput)
	if err != nil {
		return err
	}
	defer func() {
		agent.Close()
	}()
	monitoring.AddCheckers(agent, monitoringConf)
	err = agent.Start()
	if err != nil {
		return err
	}
	if monitoringConf.Role == monitoring.RoleNode {
		noReplay := false
		agent.Join([]string{join}, noReplay)
	}
	return handleAgentSignals(agent)
}

func clusterStatus(rpcAddr string) (ok bool, err error) {
	client, err := agent.NewClient(rpcAddr)
	if err != nil {
		return false, trace.Wrap(err)
	}
	status, err := client.Status()
	if err != nil {
		return false, trace.Wrap(err)
	}
	ok = status.SystemStatus == health.SystemStatusRunning
	statusJson, err := json.Marshal(status)
	if err != nil {
		return ok, trace.Wrap(err, "failed to marshal status data")
	}
	if _, err = os.Stdout.Write(statusJson); err != nil {
		return ok, trace.Wrap(err, "failed to output status")
	}
	return ok, nil
}

func handleAgentSignals(agent agent.Agent) error {
	signalc := make(chan os.Signal, 2)
	signal.Notify(signalc, os.Interrupt, syscall.SIGTERM)

	select {
	case <-signalc:
		return agent.Close()
	case <-agent.ShutdownCh():
		return nil
	}
}
