package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"time"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/planet/lib/leader"
	"github.com/gravitational/planet/lib/utils"
)

// AgentConfig represents agent configuration
type AgentConfig struct {
	// PublicIP is the ip as seen by other peers
	PublicIP string
	// LeaderKey is the EtcdKey of the leader
	LeaderKey string
	// Role is the server role (e.g. node, or master)
	Role string
	// Term is the TTL of the lease before it expires if the server
	// fails to renew it
	Term time.Duration
	// EtcdEndpoints is a list of Etcd servers to connect to
	EtcdEndpoints []string
	// APIServerDNS is a name of the API server entry to lookup
	// for the currnent active API server
	APIServerDNS string
}

// String returns string representation of the agent config
func (a AgentConfig) String() string {
	return fmt.Sprintf("agent(key=%v, ip=%v, role=%v, term=%v, endpoints=%v, apiserverDNS=%v)",
		a.LeaderKey, a.PublicIP, a.Role, a.Term, a.EtcdEndpoints, a.APIServerDNS)
}

// agent starts a special module that watches the changes
func agent(a AgentConfig) error {
	log.Infof("%v start", a)
	client, err := leader.NewClient(leader.Config{Endpoints: a.EtcdEndpoints})
	if err != nil {
		return trace.Wrap(err)
	}
	if a.Role == RoleMaster {
		if err := client.AddVoter(a.LeaderKey, a.PublicIP, a.Term); err != nil {
			return trace.Wrap(err)
		}
		// certain units must work only if the node is a master
		client.AddWatchCallback(a.LeaderKey, a.Term/3, func(key, prevVal, newVal string) {
			if newVal == a.PublicIP {
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
	client.AddWatchCallback(a.LeaderKey, a.Term/3, func(key, prevVal, newVal string) {
		log.Infof("about to set %v to %v in /etc/hosts", a.LeaderKey, newVal)
		if err := utils.UpsertHostsFile(a.APIServerDNS, newVal, ""); err != nil {
			log.Errorf("failed to set hosts file: %v", err)
		}
	})

	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, os.Interrupt, os.Kill)

	// Block until a signal is received.
	s := <-sigC
	fmt.Println("Got signal:", s)
	return nil
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
