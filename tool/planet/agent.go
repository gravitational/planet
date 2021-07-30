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
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/monitoring"
	"github.com/gravitational/planet/lib/utils"

	etcdconf "github.com/gravitational/coordinate/v4/config"
	"github.com/gravitational/coordinate/v4/leader"
	"github.com/gravitational/satellite/agent"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/cmd"
	k8smembership "github.com/gravitational/satellite/lib/membership/kubernetes"
	"github.com/gravitational/satellite/lib/rpc/client"
	agentutils "github.com/gravitational/satellite/utils"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
	etcd "go.etcd.io/etcd/client"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/clientcmd"
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
	// HighAvailability enables kubernetes high availability mode.
	HighAvailability bool
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

	if err = upsertCgroups(conf.Role == RoleMaster); err != nil {
		return nil, trace.Wrap(err)
	}

	if err = conf.ETCD.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	etcdClient, err := conf.ETCD.NewClient()
	if err != nil {
		return nil, trace.Wrap(err)
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

	// Watch for changes to the leader key.
	// Update coredns.hosts with new leader address.
	client.AddWatchCallback(conf.LeaderKey, updateDNS())

	if conf.Role != RoleMaster {
		return client, nil
	}

	// Watch for changes in the election key.
	// Start/Stop voter participation when election is enabled/disabled.
	client.AddWatchCallback(conf.ElectionKey, manageParticipation(client, config, errorC))

	// Watch for changes to the leader key.
	// If this agent becomes the leader, record a LeaderElected event to the local timeline.
	client.AddWatchCallback(conf.LeaderKey, recordElectionEvents(conf.PublicIP, agent))

	if !conf.HighAvailability {
		// Watch for changes to the leader key.
		// Start/Stop control plane units when a new leader is elected.
		client.AddWatchCallback(conf.LeaderKey, manageUnits(config))
	}

	// Set initial value of election mode
	etcdapi := etcd.NewKeysAPI(etcdClient)
	_, err = etcdapi.Set(context.TODO(), conf.ElectionKey,
		strconv.FormatBool(conf.ElectionEnabled),
		&etcd.SetOptions{PrevExist: etcd.PrevNoExist})
	if !trace.IsAlreadyExists(convertError(err)) {
		return nil, trace.Wrap(err)
	}

	return client, nil
}

// manageParticipation starts voter participation when the election is enabled
// and stops voter participation when election is disabled.
func manageParticipation(client *leader.Client, config agentConfig, errorC chan error) leader.CallbackFn {
	return func(ctx context.Context, key, prevElectionMode, newElectionMode string) {
		enabled, _ := strconv.ParseBool(newElectionMode)
		switch enabled {
		case true:
			log.Info("Enable election participation.")
			// start election participation
			client.AddVoter(ctx, config.leader.LeaderKey, config.leader.PublicIP, config.leader.Term)

			// While running with HA enabled, start units when election is re-enabled
			if config.leader.HighAvailability {
				if err := startUnits(ctx); err != nil {
					log.WithError(err).Warn("Failed to start units.")
				}
				if err := validateKubernetesService(ctx, config.serviceCIDR); err != nil {
					log.WithError(err).Warn("Failed to validate kubernetes service.")
				}
			}

			return
		case false:
			log.Info("Disable election participation.")
			client.RemoveVoter(ctx, config.leader.LeaderKey, config.leader.PublicIP, config.leader.Term)

			log.Info("Shut down services until election has been re-enabled.")
			// Shut down services if we've been requested to not participate in elections
			if err := stopUnits(ctx); err != nil {
				log.WithError(err).Warn("Failed to stop units.")
			}
		}
	}
}

// manageUnits starts the control plane components on the newly elected leader
// node and stops the components on all other master nodes.
func manageUnits(config agentConfig) leader.CallbackFn {
	return func(ctx context.Context, key, prevLeaderIP, newLeaderIP string) {
		log.WithField("addr", newLeaderIP).Info("New leader.")
		if newLeaderIP != config.leader.PublicIP {
			if err := stopUnits(ctx); err != nil {
				log.WithError(err).Warn("Failed to stop units.")
			}
			return
		}
		if err := startUnits(ctx); err != nil {
			log.WithError(err).Warn("Failed to start units.")
		}
		if err := validateKubernetesService(ctx, config.serviceCIDR); err != nil {
			log.WithError(err).Warn("Failed to validate kubernetes service.")
		}
	}
}

// recordElectionEvents records a new LeaderElected event when this agent
// is elected to be the new leader.
func recordElectionEvents(publicIP string, agent agent.Agent) leader.CallbackFn {
	return func(ctx context.Context, key, prevLeaderIP, newLeaderIP string) {
		// recordEventTimeout specifies the max timeout to record an event
		const recordEventTimeout = 10 * time.Second

		if newLeaderIP != publicIP {
			return
		}

		// Ignore if same leader is re-elected
		if newLeaderIP == prevLeaderIP {
			return
		}

		ctx, cancel := context.WithTimeout(ctx, recordEventTimeout)
		defer cancel()

		agent.RecordLocalEvents(ctx, []*pb.TimelineEvent{
			pb.NewLeaderElected(agent.GetConfig().Clock.Now(), prevLeaderIP, newLeaderIP),
		})
	}
}

func writeLocalLeader(target string, masterIP string) error {
	contents := fmt.Sprintf("%s %s %s %s %s\n",
		masterIP,
		constants.APIServerDNSName,
		constants.APIServerDNSNameGravity,
		constants.RegistryDNSName,
		LegacyAPIServerDNSName)
	err := ioutil.WriteFile(
		target,
		[]byte(contents),
		SharedFileMask,
	)
	return trace.Wrap(err)
}

func updateDNS() leader.CallbackFn {
	return func(_ context.Context, key, prevLeaderIP, newLeaderIP string) {
		log.Infof("Setting new leader address to %v in %v", newLeaderIP, CoreDNSHosts)
		if err := writeLocalLeader(CoreDNSHosts, newLeaderIP); err != nil {
			log.WithError(err).Error("Failed to update DNS configuration.")
		}
	}
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

// getKubeClientFromPath returns a Kubernetes clientset using the given
// kubeconfig file path.
func getKubeClientFromPath(kubeconfigPath string) (*kubernetes.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return client, nil
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

	err := config.monitoring.CheckAndSetDefaults()
	if err != nil {
		return trace.Wrap(err)
	}
	if config.agent.Tags == nil {
		config.agent.Tags = make(map[string]string)
	}
	config.agent.Tags["role"] = string(config.monitoring.Role)

	clientset, err := getKubeClientFromPath(constants.KubeletConfigPath)
	if err != nil {
		return trace.Wrap(err, "failed to get Kubernetes clientset")
	}

	informer := informers.NewSharedInformerFactory(clientset, 0).Core().V1().Nodes().Informer()
	stop := make(chan struct{})
	defer close(stop)
	go informer.Run(stop)

	cluster, err := k8smembership.NewCluster(&k8smembership.Config{
		Informer: informer,
	})
	if err != nil {
		return trace.Wrap(err, "failed to initialize cluster membership")
	}
	config.agent.Cluster = cluster

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

	errorC := make(chan error, 10)
	client, err := startLeaderClient(config, monitoringAgent, errorC)
	if err != nil {
		return trace.Wrap(err)
	}
	defer client.Close()

	if config.monitoring.Role == agent.RoleMaster {
		err = ensureDNSServices(ctx, config.serviceCIDR)
		if err != nil {
			return trace.Wrap(err)
		}
	}

	err = setupResolver(ctx, config.monitoring.Role, config.serviceCIDR)
	if err != nil {
		return trace.Wrap(err)
	}

	// Only non-masters run etcd gateway service
	if config.monitoring.Role != RoleMaster {
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
			if agentutils.IsUnavailableError(err) {
				return false, newAgentUnavailableError()
			}
			return false, trace.Wrap(err)
		}
		ok = status.Status == pb.NodeStatus_Running
		statusBlob = status
	} else {
		status, err := client.Status(ctx)
		if err != nil {
			if agentutils.IsUnavailableError(err) {
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

// validateKubernetesService ensures that API server service has the IP from the
// currently active service CIDR. In case of this not being the case (eg during service CIDR updates),
// the function will remove the service and wait for Kubernetes API server to recreate it.
func validateKubernetesService(ctx context.Context, serviceCIDR net.IPNet) error {
	logger := log.WithField("service-cidr", serviceCIDR.String())
	return utils.RetryWithInterval(ctx, newUnlimitedExponentialBackoff(5*time.Second), func() error {
		client, err := cmd.GetKubeClientFromPath(constants.SchedulerConfigPath)
		if err != nil {
			return trace.Wrap(err)
		}
		services := client.CoreV1().Services(metav1.NamespaceDefault)
		svc, err := services.Get(ctx, KubernetesServiceName, metav1.GetOptions{})
		if err != nil {
			return trace.Wrap(err)
		}
		if svc.Spec.ClusterIP == "" {
			// not a clusterIP service?
			logger.Info("API server service clusterIP is empty.")
			return nil
		}
		clusterIP := net.ParseIP(svc.Spec.ClusterIP)
		// Kubernetes defaults to the first IP in service range for the API server service
		// See https://github.com/kubernetes/kubernetes/blob/v1.15.12/pkg/master/services.go#L41
		//
		// Check the existing service against the active service IP range
		// and remove it if it's invalid letting the kubernetes API server recreate the service.
		// Note, this only runs on the leader node
		if !serviceCIDR.Contains(clusterIP) {
			logger.WithField("cluster-ip", svc.Spec.ClusterIP).
				Warn("API server service clusterIP is invalid, will reset service.")
			if err := removeKubernetesService(ctx, services); err != nil {
				return trace.Wrap(err)
			}
			return utils.Continue("wait for API server service to become available")
		}
		logger.Info("API server service clusterIP is valid.")
		return nil
	})
}

func removeKubernetesService(ctx context.Context, services corev1.ServiceInterface) error {
	err := services.Delete(ctx, KubernetesServiceName, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return trace.Wrap(err)
	}
	if errors.IsNotFound(err) {
		log.Info("API server service not found and will be recreated shortly.")
	}
	return nil
}

func rpcAddr(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}

func newAgentUnavailableError() error {
	return trace.LimitExceeded("agent could not be contacted. Make sure that the planet-agent service is running and try again")
}
