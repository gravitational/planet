package main

import (
	"encoding/json"
	"os"
	"os/signal"
	"syscall"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/planet/lib/agent"
	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
	"github.com/gravitational/planet/lib/monitoring"
)

type agentRole monitoring.Role

func runAgent(conf *agent.Config, monitoringConf *monitoring.Config, peers []string) error {
	if conf.Tags == nil {
		conf.Tags = make(map[string]string)
	}
	conf.Tags["role"] = string(monitoringConf.Role)
	agent, err := agent.New(conf)
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
	if len(peers) > 0 {
		err = agent.Join(peers)
		if err != nil {
			return trace.Wrap(err, "failed to join serf cluster")
		}
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
	ok = status.Status == pb.StatusType_SystemRunning
	statusJson, err := json.MarshalIndent(status, "", "   ")
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
	}
}
