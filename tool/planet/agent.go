package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/gravitational/planet/lib/etcdconf"
	"github.com/gravitational/planet/lib/leader"
	"github.com/gravitational/planet/lib/monitoring"
	"github.com/gravitational/planet/lib/utils"
	"github.com/gravitational/satellite/agent"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/trace"

	log "github.com/Sirupsen/logrus"
	"golang.org/x/net/context"
)

// LeaderConfig represents configuration for the master election task
type LeaderConfig struct {
	// PublicIP is the ip as seen by other peers
	PublicIP string
	// LeaderKey is the EtcdKey of the leader
	LeaderKey string
	// Role is the server role (e.g. node, or master)
	Role string
	// Term is the TTL of the lease before it expires if the server
	// fails to renew it
	Term time.Duration
	// ETCD defines etcd configuration
	ETCD etcdconf.Config
	// APIServerDNS is a name of the API server entry to lookup
	// for the currently active API server
	APIServerDNS string
}

// String returns string representation of the agent leader configuration
func (conf LeaderConfig) String() string {
	return fmt.Sprintf("LeaderConfig(key=%v, ip=%v, role=%v, term=%v, endpoints=%v, apiserverDNS=%v)",
		conf.LeaderKey, conf.PublicIP, conf.Role, conf.Term, conf.ETCD.Endpoints, conf.APIServerDNS)
}

// startLeaderClient starts the master election loop and sets up callbacks
// that handle state (master <-> node) changes.
//
// When a node becomes the active master, it starts a set of services specific to a master.
// Otherwise, the services are stopped to avoid interfering with the active master instance.
// Also, every time a new master is elected, the node modifies its /etc/hosts file
// to reflect the change of the kubernetes API server.
func startLeaderClient(conf *LeaderConfig) (io.Closer, error) {
	log.Infof("%v start", conf)
	client, err := leader.NewClient(leader.Config{ETCD: conf.ETCD})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if conf.Role == RoleMaster {
		if err := client.AddVoter(conf.LeaderKey, conf.PublicIP, conf.Term); err != nil {
			return nil, trace.Wrap(err)
		}
		// certain units must work only if the node is a master
		client.AddWatchCallback(conf.LeaderKey, conf.Term/3, func(key, prevVal, newVal string) {
			if newVal == conf.PublicIP {
				log.Infof("is a leader, start units")
				if err := unitsCommand("start"); err != nil {
					log.Infof("failed to execute: %v", err)
				}
			} else {
				log.Infof("%v just became a new leader", newVal)
				if err := unitsCommand("stop"); err != nil {
					log.Infof("failed to execute: %v", err)
				}
			}
		})
	}
	// modify /etc/hosts with new apiserver
	client.AddWatchCallback(conf.LeaderKey, conf.Term/3, func(key, prevVal, newVal string) {
		log.Infof("about to set %v to %v in /etc/hosts", conf.LeaderKey, newVal)
		if err := utils.UpsertHostsFile(conf.APIServerDNS, newVal, ""); err != nil {
			log.Errorf("failed to set hosts file: %v", err)
		}
	})

	return client, nil
}

var electedUnits = []string{"kube-controller-manager", "kube-scheduler"}

func unitsCommand(command string) error {
	log.Infof("about to execute %v on %v", command, electedUnits)
	for _, unit := range electedUnits {
		cmd := exec.Command("/bin/systemctl", command, fmt.Sprintf("%v.service", unit))
		log.Infof("about to execute command: %v", cmd)
		if err := cmd.Run(); err != nil {
			return trace.Wrap(err, "error %v", cmd)
		}
	}
	return nil
}

// runAgent starts the master election / health check loops in background and
// blocks until a signal has been received.
func runAgent(conf *agent.Config, monitoringConf *monitoring.Config, leaderConf *LeaderConfig, peers []string) error {
	if conf.Tags == nil {
		conf.Tags = make(map[string]string)
	}
	conf.Tags["role"] = string(monitoringConf.Role)
	monitoringAgent, err := agent.New(conf)
	if err != nil {
		return trace.Wrap(err)
	}
	defer monitoringAgent.Close()
	monitoring.AddCheckers(monitoringAgent, monitoringConf)
	err = monitoringAgent.Start()
	if err != nil {
		return trace.Wrap(err)
	}
	if len(peers) > 0 {
		err = monitoringAgent.Join(peers)
		if err != nil {
			return trace.Wrap(err, "failed to join serf cluster")
		}
	}

	client, err := startLeaderClient(leaderConf)
	if err != nil {
		return trace.Wrap(err)
	}
	defer client.Close()

	if monitoringConf.Role == agent.RoleMaster {
		dns := &DNSBootstrapper{
			clusterIP: monitoringConf.ClusterDNS,
			kubeAddr:  monitoringConf.KubeAddr,
			agent:     monitoringAgent,
		}
		go dns.create()
	}

	signalc := make(chan os.Signal, 2)
	signal.Notify(signalc, os.Interrupt, syscall.SIGTERM)

	select {
	case <-signalc:
	}

	return nil
}

// statusTimeout is the maximum time status query is blocked.
const statusTimeout = 5 * time.Second

// status obtains either the status of the planet cluster or that of
// the local node from the local planet agent.
func status(RPCPort int, local, prettyPrint bool) (ok bool, err error) {
	RPCAddr := fmt.Sprintf("127.0.0.1:%d", RPCPort)
	client, err := agent.NewClient(RPCAddr)
	if err != nil {
		return false, trace.Wrap(err)
	}
	var statusJson []byte
	var statusBlob interface{}
	ctx, cancel := context.WithTimeout(context.Background(), statusTimeout)
	defer cancel()
	if local {
		status, err := client.LocalStatus(ctx)
		if err != nil {
			return false, trace.Wrap(err)
		}
		ok = status.Status == pb.NodeStatus_Running
		statusBlob = status
	} else {
		status, err := client.Status(ctx)
		if err != nil {
			return false, trace.Wrap(err)
		}
		ok = status.Status == pb.SystemStatus_Running
		statusBlob = status
	}
	if prettyPrint {
		statusJson, err = json.MarshalIndent(statusBlob, "", "   ")
	} else {
		statusJson, err = json.Marshal(statusBlob)
	}
	if err != nil {
		return ok, trace.Wrap(err, "failed to marshal status data")
	}
	if _, err = os.Stdout.Write(statusJson); err != nil {
		return ok, trace.Wrap(err, "failed to output status")
	}
	return ok, nil
}
