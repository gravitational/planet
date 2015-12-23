package main

import (
	stdlog "log"
	"os"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/gopkg.in/alecthomas/kingpin.v2"
	"github.com/gravitational/trace"
)

func main() {
	if err := run(); err != nil {
		stdlog.Fatalln(err)
	}
}

func run() error {
	var (
		app = kingpin.New("agent", "Agent is a planet service to control and test a running cluster")

		debug = app.Flag("debug", "verbose mode").Default("true").Bool()

		cagent = app.Command("agent", "run the planet agent")
		// FIXME: wrap as HostPort
		cagentBindAddr = cagent.Flag("bind-addr", "address to bind network listeners to.  To use an IPv6 address, specify [::1] or [::1]:7946.").Default("0.0.0.0:7946").String()
		cagentRPCAddr  = cagent.Flag("rpc-addr", "address to bind the RPC listener").Default("127.0.0.1:7373").String()
		cagentJoin     = cagent.Flag("join", "address of the agent to join").String()
		cagentMode     = cagent.Flag("mode", "agent operating mode (master/node)").String()

		cstatus     = app.Command("status", "query the state of the running cluster")
		cstatusAddr = cstatus.Flag("addr", "agent RPC address").Default("127.0.0.1:7373").String()
	)

	var err error
	var cmd string
	cmd, err = app.Parse(os.Args[1:])

	if *debug == true {
		log.Initialize("console", "INFO")
	} else {
		log.Initialize("console", "WARN")
	}

	switch cmd {
	case cagent.FullCommand():
		conf := &config{
			bindAddr: *cagentBindAddr,
			rpcAddr:  *cagentRPCAddr,
			mode:     agentMode(*cagentMode),
		}
		err = runAgent(conf, *cagentJoin)
	case cstatus.FullCommand():
		err = status(*cstatusAddr)
	}

	return err
}

func runAgent(conf *config, join string) error {
	logOutput := os.Stderr
	agent, err := newAgent(conf, logOutput)
	if err != nil {
		return err
	}
	defer func() {
		agent.Leave()
		agent.Shutdown()
	}()
	conn, err := agent.start()
	if err != nil {
		return err
	}
	defer conn.Shutdown()
	if conf.mode == node {
		noReplay := false
		agent.Join([]string{join}, noReplay)
	}
	select {
	case <-agent.ShutdownCh():
	}
	return nil
}

func status(rpcAddr string) error {
	node, err := os.Hostname()
	if err != nil {
		return trace.Wrap(err)
	}
	client, err := newClient(node, rpcAddr)
	if err != nil {
		return trace.Wrap(err)
	}
	clusterStatus, err := client.status()
	if err != nil {
		return trace.Wrap(err)
	}
	log.Infof("%v", clusterStatus)
	return nil
}
