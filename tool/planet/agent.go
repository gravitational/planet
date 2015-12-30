package main

import (
	"encoding/json"
	"os"
	"os/signal"
	"syscall"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/planet/lib/agent"
	"github.com/gravitational/planet/lib/agent/monitoring"
)

func runAgent(conf *agent.Config, join string) error {
	logOutput := os.Stderr
	testAgent, err := agent.NewAgent(conf, logOutput)
	if err != nil {
		return err
	}
	defer func() {
		testAgent.Leave()
		testAgent.Shutdown()
	}()
	conn, err := testAgent.Start()
	if err != nil {
		return err
	}
	defer conn.Shutdown()
	if conf.Mode == agent.Node {
		noReplay := false
		testAgent.Join([]string{join}, noReplay)
	}
	return handleAgentSignals(testAgent)
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
	ok = status.SystemStatus == monitoring.SystemStatusRunning
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
		agent.Leave()
		return agent.Shutdown()
	case <-agent.ShutdownCh():
		return nil
	}
}
