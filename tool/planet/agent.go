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
	"io/ioutil"
	"math"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/monitoring"
	"github.com/gravitational/planet/lib/utils"

	etcd "github.com/coreos/etcd/client"
	etcdconf "github.com/gravitational/coordinate/config"
	"github.com/gravitational/coordinate/leader"
	"github.com/gravitational/satellite/agent"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/cmd"
	"github.com/gravitational/satellite/lib/rpc/client"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
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
func startLeaderClient(config agentConfig, agent agent.Agent, errorC chan error) (leaderClient io.Closer, err error) {
	conf := config.leader
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
	// services as a leader.
	client.AddWatchCallback(conf.LeaderKey, conf.Term/3, func(key, prevLeaderAddr, newLeaderAddr string) {
		log.WithField("addr", newLeaderAddr).Info("New leader.")
		if newLeaderAddr != conf.PublicIP {
			if err := stopUnits(context.TODO()); err != nil {
				log.WithError(err).Warn("Failed to stop units.")
			}
			return
		}
		if err := startUnits(context.TODO()); err != nil {
			log.WithError(err).Warn("Failed to start units.")
		}
		if err := validateKubernetesService(context.TODO(), config.serviceCIDR); err != nil {
			log.WithError(err).Warn("Failed to validate kubernetes service.")
		}
	})

	// Add a callback to watch for changes to the leader key.
	// If this node becomes the leader, record a LeaderElected event to the
	// local timeline.
	client.AddWatchCallback(conf.LeaderKey, conf.Term/3, func(key, prevVal, newVal string) {
		// recordEventTimeout specifies the max timeout to record an event
		const recordEventTimeout = 10 * time.Second

		if newVal != conf.PublicIP {
			return
		}
		// Ignore if same leader is re-elected
		if newVal == prevVal {
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), recordEventTimeout)
		defer cancel()

		agent.RecordLocalEvents(ctx, []*pb.TimelineEvent{
			pb.NewLeaderElected(agent.GetConfig().Clock.Now(), prevVal, newVal),
		})
	})

	var cancelVoter context.CancelFunc
	var ctx context.Context
	if conf.Role == RoleMaster {
		switch conf.ElectionEnabled {
		case true:
			log.Infof("Adding voter for IP %v.", conf.PublicIP)
			ctx, cancelVoter = context.WithCancel(context.TODO())
			if err = client.AddVoter(ctx, conf.LeaderKey, conf.PublicIP, conf.Term); err != nil {
				return nil, trace.Wrap(err)
			}
		case false:
			log.Info("Shut down services until election has been re-enabled.")
			// Shut down services at startup if running as master
			if err := stopUnits(context.TODO()); err != nil {
				log.WithError(err).Warn("Failed to stop units.")
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
	contents := fmt.Sprint(masterIP, " ",
		constants.APIServerDNSName, " ",
		constants.APIServerDNSNameGravity, " ",
		constants.RegistryDNSName, " ",
		LegacyAPIServerDNSName, "\n")
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

var controlPlaneUnits = []string{
	"kube-controller-manager.service",
	"kube-scheduler.service",
	"kube-apiserver.service",
}

func startUnits(ctx context.Context) error {
	log.Debug("Start control plane units.")
	var errors []error
	for _, unit := range controlPlaneUnits {
		logger := log.WithField("unit", unit)
		err := systemctlCmd(ctx, "start", unit)
		if err != nil {
			errors = append(errors, err)
			// Instead of failing immediately, complete start of other units
			logger.WithError(err).Warn("Failed to start unit.")
		}
	}
	return trace.NewAggregate(errors...)
}

func stopUnits(ctx context.Context) error {
	log.Debug("Stop control plane units.")
	var errors []error
	for _, unit := range controlPlaneUnits {
		logger := log.WithField("unit", unit)
		err := systemctlCmd(ctx, "stop", unit)
		if err != nil {
			errors = append(errors, err)
			// Instead of failing immediately, complete stop of other units
			logger.WithError(err).Warn("Failed to stop unit.")
		}
		// Even if 'systemctl stop' did not fail, the service could have failed stopping
		// even though 'stop' is blocking, it does not return an error upon service failing.
		// See github.com/gravitational/gravity/issues/1209 for more details
		if err := systemctlCmd(ctx, "is-failed", unit); err == nil {
			logger.Info("Reset failed unit.")
			if err := systemctlCmd(ctx, "reset-failed", unit); err != nil {
				logger.WithError(err).Warn("Failed to reset failed unit.")
			}
		}
	}
	return trace.NewAggregate(errors...)
}

type agentConfig struct {
	agent       *agent.Config
	monitoring  *monitoring.Config
	leader      *LeaderConfig
	peers       []string
	serviceCIDR net.IPNet
}

// runAgent starts the master election / health check loops in background and
// blocks until a signal has been received.
func runAgent(config agentConfig) error {
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	if config.agent.Tags == nil {
		config.agent.Tags = make(map[string]string)
	}
	config.agent.Tags["role"] = string(config.monitoring.Role)
	monitoringAgent, err := agent.New(config.agent)
	if err != nil {
		return trace.Wrap(err)
	}
	defer monitoringAgent.Close()

	err = monitoring.AddMetrics(monitoringAgent, config.monitoring)
	if err != nil {
		return trace.Wrap(err)
	}
	err = monitoring.AddCheckers(monitoringAgent, config.monitoring)
	if err != nil {
		return trace.Wrap(err)
	}
	err = monitoringAgent.Start()
	if err != nil {
		return trace.Wrap(err)
	}

	// only join to the initial seed list if not member already,
	// as the initial peer could be gone
	if !monitoringAgent.IsMember() && len(config.peers) > 0 {
		log.Infof("joining the cluster: %v", config.peers)
		err = monitoringAgent.Join(config.peers)
		if err != nil {
			return trace.Wrap(err, "failed to join serf cluster")
		}
	} else {
		log.Info("this agent is already a member of the cluster")
	}

	errorC := make(chan error, 10)
	client, err := startLeaderClient(config, monitoringAgent, errorC)
	if err != nil {
		return trace.Wrap(err)
	}
	defer client.Close()

	err = setupResolver(ctx, config.monitoring.Role, config.serviceCIDR)
	if err != nil {
		return trace.Wrap(err)
	}

	// Only non-masters run etcd gateway service
	if config.leader.Role != RoleMaster {
		err = startWatchingEtcdMasters(ctx, config.monitoring)
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

func validateKubernetesService(ctx context.Context, serviceCIDR net.IPNet) error {
	return utils.Retry(ctx, math.MaxInt64, 5*time.Second, func() error {
		client, err := cmd.GetKubeClientFromPath(constants.SchedulerConfigPath)
		if err != nil {
			return trace.Wrap(err)
		}
		services := client.CoreV1().Services(metav1.NamespaceDefault)
		svc, err := services.Get(KubernetesServiceName, metav1.GetOptions{})
		if err != nil {
			return trace.Wrap(err)
		}
		if svc.Spec.ClusterIP == "" {
			// not a clusterIP service?
			log.Info("kubernetes service clusterIP is empty.")
			return nil
		}
		clusterIP := net.ParseIP(svc.Spec.ClusterIP)
		if !serviceCIDR.Contains(clusterIP) {
			log.WithField("cluster-ip", svc.Spec.ClusterIP).
				Warn("kubernetes service clusterIP is invalid, will recreate service.")
			return removeKubernetesService(services)
		}
		log.Info("kubernetes service clusterIP is valid.")
		return nil
	})
}

// Kubernetes defaults to the first IP in service range for the kubernetes service
// See https://github.com/kubernetes/kubernetes/blob/v1.15.12/pkg/master/services.go#L41
//
// This checks the existing kubernetes services against the active service IP range
// and removes it if it's invalid letting the kubernetes api server to recreate it.
// Note, this only runs on the leader node
func removeKubernetesService(services corev1.ServiceInterface) error {
	err := services.Delete(KubernetesServiceName, &metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return trace.Wrap(err)
	}
	return nil
}

func rpcAddr(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}

func newAgentUnavailableError() error {
	return trace.LimitExceeded("agent could not be contacted. Make sure that the planet-agent service is running and try again")
}
