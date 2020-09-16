/*
Copyright 2020 Gravitational, Inc.

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
	"time"

	"github.com/gravitational/planet/lib/monitoring"

	"github.com/gravitational/trace"
	serf "github.com/hashicorp/serf/client"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// startSerfReconciler creates a control loop that periodically attempts to
// reconcile serf cluster.
func startSerfReconciler(ctx context.Context, serfConfig *serf.Config) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			k8sPeers, err := getK8sPeers()
			if err != nil {
				log.WithError(err).Warn("Failed to query kubernetes peers.")
				break
			}

			serfPeers, err := getSerfPeers(serfConfig)
			if err != nil {
				log.WithError(err).Warn("Failed to query serf peers.")
				break
			}

			if !shouldReconcileSerf(k8sPeers, serfPeers) {
				break
			}

			log.WithField("k8s-peers", k8sPeers).
				WithField("serf-peers", serfPeers).
				Info("Reconciling serf cluster.")

			numJoined, err := reconcileSerfCluster(serfConfig, k8sPeers)
			if err != nil {
				log.WithError(err).Warn("Failed to reconcile serf cluster.")
				break
			}
			log.WithField("num-joined", numJoined).Info("Reconciled serf cluster.")

		case <-ctx.Done():
			log.Debug("Stopping serf reconciler")
			return
		}
	}
}

// getK8sPeers returns advertised IP addresses of all nodes in the kubernetes
// cluster.
func getK8sPeers() (peers []string, err error) {
	client, err := monitoring.GetKubeClient()
	if err != nil {
		return peers, trace.Wrap(err, "failed to get kubernetes clientset")
	}
	nodes, err := client.CoreV1().Nodes().List(metav1.ListOptions{
		LabelSelector: advertiseIPKey,
	})
	if err != nil {
		return peers, trace.Wrap(err, "failed to list kubernetes nodes")
	}

	for _, node := range nodes.Items {
		addr, err := getAddr(node)
		if err != nil {
			return peers, trace.Wrap(err, "failed to get advertised IP address of %s", node.Name)
		}
		peers = append(peers, addr)
	}
	return peers, nil
}

// getAddr returns the advertised IP address of the node.
func getAddr(node v1.Node) (addr string, err error) {
	addr, exists := node.Labels[advertiseIPKey]
	if !exists {
		return addr, trace.NotFound("node %s does not have %s label", node.Name, advertiseIPKey)
	}
	return addr, nil
}

// getSerfPeers returns the advertised IP address of all nodes in the serf
// cluster.
func getSerfPeers(config *serf.Config) (peers []string, err error) {
	client, err := serf.ClientFromConfig(config)
	if err != nil {
		return peers, trace.Wrap(err, "failed to create serf client")
	}
	defer client.Close()

	members, err := client.Members()
	if err != nil {
		return peers, trace.Wrap(err, "failed to list serf members")
	}
	for _, member := range members {
		peers = append(peers, member.Addr.String())
	}
	return peers, nil
}

// shouldReconcileSerf returns true if a member has been partitioned off the
// serf cluster.
func shouldReconcileSerf(k8sPeers, serfPeers []string) bool {
	// missing tracks the members missing from the serf cluster.
	missing := make(map[string]struct{})
	for _, k8sPeer := range k8sPeers {
		missing[k8sPeer] = struct{}{}
	}
	for _, serfPeer := range serfPeers {
		delete(missing, serfPeer)
	}
	return len(missing) != 0
}

// reconcileSerfCluster attempts to reconcile the serf cluster.
func reconcileSerfCluster(config *serf.Config, peers []string) (joined int, err error) {
	client, err := serf.ClientFromConfig(config)
	if err != nil {
		return joined, trace.Wrap(err, "failed to create serf client")
	}
	defer client.Close()

	joined, err = client.Join(peers, false)
	if err != nil {
		return joined, trace.Wrap(err, "failed to join serf cluster")
	}
	return joined, nil
}

// advertiseIPKey specifies the key mapped to the advertised IP address.
const advertiseIPKey = "gravitational.io/advertise-ip"
