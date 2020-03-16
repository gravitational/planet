/*
Copyright 2018 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gravitational/planet/lib/monitoring"
	"github.com/gravitational/planet/lib/reconcile"

	etcdconf "github.com/gravitational/coordinate/config"
	"github.com/gravitational/coordinate/leader"
	"github.com/gravitational/satellite/agent"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/rpc/client"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
	etcd "go.etcd.io/etcd/client"
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
func startLeaderClient(ctx context.Context, conf *LeaderConfig, agent agent.Agent, errorC chan error) (leaderClient io.Closer, err error) {
	log.Infof("%v start", conf)
	var hostname string
	hostname, err = os.Hostname()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	if conf.Role == RoleMaster {
		err = upsertCgroups(true)
	} else {
		err = upsertCgroups(false)
	}
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

	client, err := leader.NewClient(leader.Config{Client: etcdClient})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	defer func() {
		if err != nil {
			client.Close()
		}
	}()

	logger := log.WithField("addr", conf.PublicIP)

	if conf.Role == RoleMaster {
		etcdapi := etcd.NewKeysAPI(etcdClient)
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

	mon, err := newUnitMonitor()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	plan := agentPlan{
		leaderKey:    conf.LeaderKey,
		localAddr:    conf.PublicIP,
		electionTerm: conf.Term,
		mon:          mon,
		client:       client,
	}
	reconciler := reconcile.New()

	client.AddWatchCallback(conf.LeaderKey, conf.Term/3, updateLeaderTrigger(ctx, reconciler, plan, logger))
	client.AddWatchCallback(conf.ElectionKey, conf.Term,
		updateElectionParticipationTrigger(ctx, reconciler, plan, logger))

	return &clientState{
		client:     client,
		reconciler: reconciler,
	}, nil
}

// Close closes this state by releasing the resources.
// Implements io.Closer
func (r *clientState) Close() error {
	r.reconciler.Stop()
	return r.client.Close()
}

type clientState struct {
	client     *leader.Client
	reconciler *reconcile.Reconciler
}

func updateLeaderTrigger(ctx context.Context, reconciler *reconcile.Reconciler, plan agentPlan, logger log.FieldLogger) leader.CallbackFn {
	return func(_, _, newLeaderAddr string) {
		logger.WithField("leader-addr", newLeaderAddr).Info("New leader.")
		plan.leaderAddr = newLeaderAddr
		reconciler.Reset(ctx, &plan)
	}
}

func updateElectionParticipationTrigger(ctx context.Context, reconciler *reconcile.Reconciler, plan agentPlan, logger log.FieldLogger) leader.CallbackFn {
	return func(_, _, enabledFlag string) {
		enabled, _ := strconv.ParseBool(enabledFlag)
		logger.WithField("enabled", enabled).Info("Election participation status.")
		plan.electionPaused = !enabled
		reconciler.Reset(ctx, &plan)
	}
}

// runAgent starts the master election / health check loops in background and
// blocks until a signal has been received.
func runAgent(conf *agent.Config, monitoringConf *monitoring.Config, leaderConf *LeaderConfig, peers []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := monitoringConf.CheckAndSetDefaults()
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

	err = monitoring.AddMetrics(monitoringAgent, monitoringConf)
	if err != nil {
		return trace.Wrap(err)
	}

	err = monitoring.AddCheckers(monitoringAgent, monitoringConf)
	if err != nil {
		return trace.Wrap(err)
	}
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
	client, err := startLeaderClient(ctx, leaderConf, monitoringAgent, errorC)
	if err != nil {
		return trace.Wrap(err)
	}
	defer client.Close()

	err = setupResolver(ctx, monitoringConf.Role)
	if err != nil {
		return trace.Wrap(err)
	}

	// Only non-masters run etcd gateway service
	if leaderConf.Role != RoleMaster {
		err = startWatchingEtcdMasters(ctx, monitoringConf)
		if err != nil {
			return trace.Wrap(err)
		}
	}

	go runSystemdCgroupCleaner(ctx)

	signalc := make(chan os.Signal, 2)
	signal.Notify(signalc, os.Interrupt, syscall.SIGTERM)

	select {
	case <-signalc:
	case err := <-errorC:
		return trace.Wrap(err)
	}

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
	ctx, cancel := context.WithTimeout(context.TODO(), c.timeout)
	defer cancel()

	config := client.Config{
		Address:  rpcAddr(c.rpcPort),
		CAFile:   c.caFile,
		CertFile: c.clientCertFile,
		KeyFile:  c.clientKeyFile,
	}

	client, err := client.NewClient(ctx, config)
	if err != nil {
		return false, trace.Wrap(err)
	}
	var statusJson []byte
	var statusBlob interface{}
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
