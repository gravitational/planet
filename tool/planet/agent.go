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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/monitoring"

	etcdconf "github.com/gravitational/coordinate/config"
	"github.com/gravitational/satellite/agent"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/ctxgroup"
	"github.com/gravitational/satellite/lib/rpc/client"
	agentutils "github.com/gravitational/satellite/utils"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
	etcd "go.etcd.io/etcd/client"
	"go.etcd.io/etcd/clientv3"
	"go.etcd.io/etcd/clientv3/concurrency"
	"go.etcd.io/etcd/mvcc/mvccpb"
	"k8s.io/client-go/tools/clientcmd"
)

// TODO: startEtcdLoop ...
func startEtcdLoop(ctx context.Context, client *clientv3.Client, addr string, g *ctxgroup.Group) error {
	hostname, err := os.Hostname()
	if err != nil {
		return trace.Wrap(err)
	}

	// TODO: recover an active session by persisting s.Lease()
	// to a file.
	// TODO: recreate the session if interrupted due to an etcd error
	s, err := concurrency.NewSession(client, concurrency.WithContext(ctx), concurrency.WithTTL(nodeTTLSeconds))
	if err != nil {
		return trace.Wrap(err, "failed to create session")
	}

	nodePath := filepath.Join(masterPrefix, addr)

	// TODO: relist watch in case of errors
	wchan := client.Watch(clientv3.WithRequireLeader(ctx), masterPrefix,
		clientv3.WithPrefix(),
	)

	g.GoCtx(func(ctx context.Context) error {
		defer s.Close()
		m := concurrency.NewLocker(s, nodePath)
		m.Lock()
		<-ctx.Done()
		m.Unlock()
		return nil
	})

	g.GoCtx(func(ctx context.Context) error {
		for {
			select {
			case u := <-wchan:
				for _, ev := range u.Events {
					if ev.Type != mvccpb.PUT && ev.Type != mvccpb.DELETE {
						continue
					}
					// TODO: implement proper skip if no relevant events
				}
				// modify CoreDNS hosts when masters are added/removed or get partitioned
				// TODO: for simplicity, requery the masters tree and snapshot the value into the hosts file
				resp, err := client.Get(ctx, "/planet/cluster/masters/", clientv3.WithPrefix(), clientv3.WithKeysOnly())
				if err != nil {
					log.WithError(err).Warn("Failed to query master nodes.")
					// TODO: retry
					continue
				}
				var addrs []string
				for _, kv := range resp.Kvs {
					key := strings.TrimPrefix(string(kv.Key), masterPrefix)
					logger := log.WithField("key", key)
					logger.Info("Look at master keys.")
					addr := strings.SplitN(key, "/", 2)[0]
					addrs = append(addrs, addr)
				}
				log.WithField("addrs", addrs).Info("Update DNS configuration.")
				if err := updateDNS(hostname, addrs); err != nil {
					log.WithError(err).Error("Failed to update DNS configuration.")
				}

			case <-ctx.Done():
				return nil
			}
		}
	})

	return nil
}

const (
	masterPrefix = "/planet/cluster/masters/"

	// nodeTTLSeconds specifies the maximum amount of time when the node lease
	// expires after the agent is shut down
	nodeTTLSeconds = 15
)

func updateDNS(hostname string, masterIPs []string) error {
	log.Infof("Setting master addresses to %v in %v", masterIPs, CoreDNSHosts)
	var buf bytes.Buffer
	for _, addr := range masterIPs {
		fmt.Fprint(&buf, addr, " ",
			constants.APIServerDNSName, " ",
			constants.APIServerDNSNameGravity, " ",
			constants.RegistryDNSName, " ",
			LegacyAPIServerDNSName, "\n")
	}
	return trace.ConvertSystemError(ioutil.WriteFile(CoreDNSHosts, buf.Bytes(), SharedFileMask))
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

// runAgent starts the master election / health check loops in background and
// blocks until a signal has been received.
func runAgent(conf agent.Config, monitoringConf monitoring.Config, etcdConf etcdconf.Config, peers []string) error {
	if err := etcdConf.CheckAndSetDefaults(); err != nil {
		return trace.Wrap(err)
	}
	err := monitoringConf.CheckAndSetDefaults()
	if err != nil {
		return trace.Wrap(err)
	}

	if conf.Tags == nil {
		conf.Tags = make(map[string]string)
	}
	conf.Tags["role"] = string(monitoringConf.Role)
	monitoringAgent, err := agent.New(&conf)
	if err != nil {
		return trace.Wrap(err)
	}

	err = monitoring.AddMetrics(monitoringAgent, &monitoringConf)
	if err != nil {
		return trace.Wrap(err)
	}

	err = monitoring.AddCheckers(monitoringAgent, &monitoringConf)
	if err != nil {
		return trace.Wrap(err)
	}

	var closers []io.Closer
	defer func() {
		if err == nil {
			return
		}
		for _, c := range closers {
			c.Close() //nolint:errcheck
		}
	}()

	err = monitoringAgent.Start()
	if err != nil {
		return trace.Wrap(err)
	}
	closers = append(closers, monitoringAgent)

	// only join to the initial seed list if not member already,
	// as the initial peer could be gone
	isMember, err := monitoringAgent.IsMember()
	if err != nil {
		return trace.Wrap(err)
	}
	if !isMember && len(peers) > 0 {
		log.Infof("joining the cluster: %v", peers)
		err = monitoringAgent.Join(peers)
		if err != nil {
			return trace.Wrap(err, "failed to join serf cluster")
		}
	} else {
		log.Info("this agent is already a member of the cluster")
	}

	if monitoringConf.Role == RoleMaster {
		err = upsertCgroups(defaultCgroupConfigMaster(runtime.NumCPU()))
	} else {
		err = upsertCgroups(defaultCgroupConfig(runtime.NumCPU()))
	}
	if err != nil {
		return trace.Wrap(err)
	}

	client, err := etcdConf.NewClientV3()
	if err != nil {
		return trace.Wrap(err)
	}
	closers = append(closers, client)

	go runSystemdCgroupCleaner(ctx)

	// FIXME: integrate
	/* 
	if leaderConf.Role == RoleMaster {
		kubeconfig, err := clientcmd.BuildConfigFromFlags("", constants.KubeletConfigPath)
		if err != nil {
			return trace.Wrap(err, "failed to build kubeconfig")
		}
		go startSerfReconciler(ctx, kubeconfig, &conf.SerfConfig)
	}
	go runSystemdCgroupCleaner(ctx)
	*/

	if err := startEtcdLoop(ctx, client, monitoringConf.AdvertiseIP, &g); err != nil {
		return trace.Wrap(err)
	}

	if monitoringConf.Role == RoleMaster {
		log.Info("Start control plane units.")
		if err := startUnits(ctx); err != nil {
			log.WithError(err).Warn("Failed to start control plane units.")
		}
	}

	err = setupResolver(ctx, monitoringConf.Role)
	if err != nil {
		return trace.Wrap(err)
	}

	// Only non-masters run etcd gateway service
	if monitoringConf.Role != RoleMaster {
		err = startWatchingEtcdMasters(ctx, monitoringConf, &g)
		if err != nil {
			return trace.Wrap(err)
		}
	}

	g.Go(monitoringAgent.Run)
	g.GoCtx(func(ctx context.Context) error {
		runSystemdCgroupCleaner(ctx)
		return nil
	})
	g.Go(func() error {
		watchSignals(cancel)
		for _, c := range closers {
			c.Close() //nolint:errcheck
		}
		return nil
	})

	return trace.Wrap(g.Wait())
}

func watchSignals(cancel context.CancelFunc) {
	defer cancel()
	signalC := make(chan os.Signal, 1)
	signal.Notify(signalC, os.Interrupt, syscall.SIGTERM, syscall.SIGUSR1)
	for sig := range signalC {
		log.WithField("signal", sig).Info("Received termination signal, will exit.")
		return
	}
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

func rpcAddr(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}

func newAgentUnavailableError() error {
	return trace.LimitExceeded("agent could not be contacted. Make sure that the planet-agent service is running and try again")
}
