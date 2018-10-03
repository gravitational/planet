package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	etcd "github.com/coreos/etcd/client"
	etcdconf "github.com/gravitational/coordinate/config"
	"github.com/gravitational/coordinate/leader"
	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/monitoring"
	"github.com/gravitational/satellite/agent"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

// LeaderConfig represents configuration for the master election task
type LeaderConfig struct {
	// PublicIP is the IP used for inter-host communication
	PublicIP string
	// LeaderKey is the EtcdKey of the leader
	LeaderKey string
	// ElectionKey is the name of the key that controls if this node
	// is participating in leader election. The value is a boolean with
	// `true` meaning active participation.
	// It is enough to update the value of this key to change the node
	// participation mode.
	ElectionKey string
	// ElectionEnabled defines the initial state of election participation
	// for this node (true == participation is on)
	ElectionEnabled bool
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
func startLeaderClient(conf *LeaderConfig, errorC chan error) (leaderClient io.Closer, err error) {
	log.Infof("%v start", conf)
	var hostname string
	hostname, err = os.Hostname()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	if err = conf.ETCD.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	var etcdClient etcd.Client
	etcdClient, err = conf.ETCD.NewClient()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	if conf.Role == RoleMaster {
		var etcdapi etcd.KeysAPI
		etcdapi = etcd.NewKeysAPI(etcdClient)
		// Set initial value of election participation mode
		_, err = etcdapi.Set(context.TODO(), conf.ElectionKey,
			strconv.FormatBool(conf.ElectionEnabled),
			&etcd.SetOptions{PrevExist: etcd.PrevNoExist})
		if err != nil {
			if err = convertError(err); !trace.IsAlreadyExists(err) {
				return nil, trace.Wrap(err)
			}
		}
	}

	client, err := leader.NewClient(leader.Config{Client: etcdClient})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	defer func() {
		if err != nil {
			log.Infof("closing client: %v", err)
			client.Close()
		}
	}()

	// Add a callback to watch for changes to the leader key.
	// If this node becomes the leader, start a number of additional
	// services as a master
	client.AddWatchCallback(conf.LeaderKey, conf.Term/3, func(key, prevVal, newVal string) {
		log.Infof("new leader: %v", newVal)
		if newVal == conf.PublicIP {
			if err := unitsCommand("start"); err != nil {
				log.Infof("failed to start units: %v", err)
			}
			return
		}
		if err := unitsCommand("stop"); err != nil {
			log.Infof("failed to stop units: %v", err)
		}
	})

	var cancelVoter context.CancelFunc
	var ctx context.Context
	if conf.Role == RoleMaster {
		switch conf.ElectionEnabled {
		case true:
			log.Infof("adding voter for IP %v", conf.PublicIP)
			ctx, cancelVoter = context.WithCancel(context.TODO())
			if err = client.AddVoter(ctx, conf.LeaderKey, conf.PublicIP, conf.Term); err != nil {
				return nil, trace.Wrap(err)
			}
		case false:
			log.Info("shutting down services until election has been re-enabled")
			// Shut down services at startup if running as master
			if err := unitsCommand("stop"); err != nil {
				log.Infof("failed to stop units: %v", err)
			}
		}
	}
	// watch the election mode status and start/stop participation
	// depending on the value of the election mode key
	client.AddWatchCallback(conf.ElectionKey, conf.Term, func(key, prevVal, newVal string) {
		var err error
		enabled, _ := strconv.ParseBool(newVal)
		switch enabled {
		case true:
			if cancelVoter != nil {
				log.Infof("voter is already active")
				return
			}
			// start election participation
			ctx, cancelVoter = context.WithCancel(context.TODO())
			if err = client.AddVoter(ctx, conf.LeaderKey, conf.PublicIP, conf.Term); err != nil {
				log.Errorf("failed to add voter for %v: %v", conf.PublicIP, trace.DebugReport(err))
				errorC <- err
			}
		case false:
			if cancelVoter == nil {
				log.Info("no voter active")
				return
			}
			// stop election participation
			cancelVoter()
			cancelVoter = nil
		}
	})
	// modify /etc/hosts upon election of a new leader node
	client.AddWatchCallback(conf.LeaderKey, conf.Term/3, func(key, prevVal, newLeaderIP string) {
		if err := updateDNS(conf, hostname, newLeaderIP); err != nil {
			log.Error(trace.DebugReport(err))
		}
	})

	return client, nil
}

func writeLocalLeader(target string, masterIP string) error {
	contents := fmt.Sprint(masterIP, " ", constants.APIServerDNSName, " ", LegacyAPIServerDNSName, "\n")
	err := ioutil.WriteFile(
		target,
		[]byte(contents),
		SharedFileMask,
	)
	return trace.Wrap(err)
}

func updateDNS(conf *LeaderConfig, hostname string, newMasterIP string) error {
	log.Infof("Setting new leader address to %v in %v", newMasterIP, CoreDNSHosts)
	err := writeLocalLeader(CoreDNSHosts, newMasterIP)
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

var electedUnits = []string{"kube-controller-manager.service", "kube-scheduler.service", "kube-apiserver.service"}

func unitsCommand(command string) error {
	log.Debugf("executing %v on %v", command, electedUnits)
	var errors []error
	for _, unit := range electedUnits {
		cmd := exec.Command("/bin/systemctl", command, unit)
		log.Debugf("executing %v", cmd)
		out, err := cmd.CombinedOutput()
		if err != nil {
			errors = append(errors, err)
			// Instead of failing immediately, complete start of other units
			log.Warningf("failed to execute %v: %s\n%v", cmd, out, trace.DebugReport(err))
		}
	}
	return trace.NewAggregate(errors...)
}

// runAgent starts the master election / health check loops in background and
// blocks until a signal has been received.
func runAgent(conf *agent.Config, monitoringConf *monitoring.Config, leaderConf *LeaderConfig, peers []string) error {
	ctx, cancelCtx := context.WithCancel(context.Background())

	err := monitoringConf.Check()
	if err != nil {
		return trace.Wrap(err)
	}
	if conf.Tags == nil {
		conf.Tags = make(map[string]string)
	}
	conf.Tags["role"] = string(monitoringConf.Role)
	monitoringAgent, err := agent.New(conf)
	if err != nil {
		return trace.Wrap(err)
	}
	defer monitoringAgent.Close()

	monitoring.AddMetrics(monitoringAgent, monitoringConf)
	monitoring.AddCheckers(monitoringAgent, monitoringConf)
	err = monitoringAgent.Start()
	if err != nil {
		return trace.Wrap(err)
	}

	// only join to the initial seed list if not member already,
	// as the initial peer could be gone
	if !monitoringAgent.IsMember() && len(peers) > 0 {
		log.Infof("joining the cluster: %v", peers)
		err = monitoringAgent.Join(peers)
		if err != nil {
			return trace.Wrap(err, "failed to join serf cluster")
		}
	} else {
		log.Info("this agent is already a member of the cluster")
	}

	errorC := make(chan error, 10)
	client, err := startLeaderClient(leaderConf, errorC)
	if err != nil {
		return trace.Wrap(err)
	}
	defer client.Close()

	err = runCoreDNSMonitor(ctx, coreDNSConfig{
		UpstreamNameservers: monitoringConf.UpstreamNameservers,
		Zones:               monitoringConf.DNSZones,
		Port:                DNSPort,
	})
	if err != nil {
		return trace.Wrap(err)
	}

	signalc := make(chan os.Signal, 2)
	signal.Notify(signalc, os.Interrupt, syscall.SIGTERM)

	select {
	case <-signalc:
	case err := <-errorC:
		return trace.Wrap(err)
	}
	cancelCtx()

	return nil
}

func leaderPause(publicIP, electionKey string, etcd *etcdconf.Config) error {
	log.Infof("disable election participation for %v", publicIP)
	return enableElection(publicIP, electionKey, false, etcd)
}

func leaderResume(publicIP, electionKey string, etcd *etcdconf.Config) error {
	log.Infof("enable election participation for %v", publicIP)
	return enableElection(publicIP, electionKey, true, etcd)
}

func leaderView(leaderKey string, etcd *etcdconf.Config) error {
	client, err := getEtcdClient(etcd)
	if err != nil {
		return trace.Wrap(err)
	}
	resp, err := client.Get(context.TODO(), leaderKey, nil)
	if err != nil {
		return trace.Wrap(err)
	}
	fmt.Println(resp.Node.Value)
	return nil
}

func enableElection(publicIP, electionKey string, enabled bool, etcd *etcdconf.Config) error {
	client, err := getEtcdClient(etcd)
	if err != nil {
		return trace.Wrap(err)
	}
	_, err = client.Set(context.TODO(), electionKey, strconv.FormatBool(enabled), nil)
	return trace.Wrap(err)
}

func getEtcdClient(conf *etcdconf.Config) (etcd.KeysAPI, error) {
	if err := conf.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	client, err := conf.NewClient()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	etcdapi := etcd.NewKeysAPI(client)
	return etcdapi, nil
}

type statusConfig struct {
	rpcPort        int
	local          bool
	prettyPrint    bool
	timeout        time.Duration
	caFile         string
	clientCertFile string
	clientKeyFile  string
}

// status obtains either the status of the planet cluster or that of
// the local node from the local planet agent.
func status(c statusConfig) (ok bool, err error) {
	client, err := agent.NewClient(rpcAddr(c.rpcPort), c.caFile, c.clientCertFile, c.clientKeyFile)
	if err != nil {
		return false, trace.Wrap(err)
	}
	var statusJson []byte
	var statusBlob interface{}
	ctx, cancel := context.WithTimeout(context.TODO(), c.timeout)
	defer cancel()
	if c.local {
		status, err := client.LocalStatus(ctx)
		if err != nil {
			if agent.IsUnavailableError(err) {
				return false, newAgentUnavailableError()
			}
			return false, trace.Wrap(err)
		}
		ok = status.Status == pb.NodeStatus_Running
		statusBlob = status
	} else {
		status, err := client.Status(ctx)
		if err != nil {
			if agent.IsUnavailableError(err) {
				return false, newAgentUnavailableError()
			}
			return false, trace.Wrap(err)
		}
		ok = status.Status == pb.SystemStatus_Running
		statusBlob = status
	}
	if c.prettyPrint {
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

func rpcAddr(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}

func newAgentUnavailableError() error {
	return trace.LimitExceeded("agent could not be contacted. Make sure that the planet-agent service is running and try again")
}
