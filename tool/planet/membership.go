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

	"github.com/gravitational/trace"
	serf "github.com/hashicorp/serf/client"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// startSerfReconciler creates a control loop that periodically attempts to
// reconcile serf cluster.
func startSerfReconciler(ctx context.Context, kubeconfig *rest.Config, serfConfig *serf.Config) {
	const reconcileInterval = time.Minute * 10
	ticker := time.NewTicker(reconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := reconcileSerfCluster(kubeconfig, serfConfig); err != nil {
				log.WithError(err).Warn("Failed to reconcile serf cluster.")
			}
		case <-ctx.Done():
			log.Debug("Stopping serf reconciler")
			return
		}
	}
}

// reconcileSerfCluster reconciles ther serf cluster if any members are missing
// from the serf cluster.
func reconcileSerfCluster(kubeconfig *rest.Config, serfConfig *serf.Config) error {
	k8sClient, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		return trace.Wrap(err, "failed to create kubernetes clientset")
	}

	serfClient, err := serf.ClientFromConfig(serfConfig)
	if err != nil {
		return trace.Wrap(err, "failed to create serf client")
	}
	defer serfClient.Close()

	return reconcileSerf(k8sClient, serfClient)
}

// reconcileSerf reconciles the serf cluster if any members are missing from the
// cluster.
func reconcileSerf(k8sClient kubernetes.Interface, serfClient serfClient) error {
	k8sPeers, err := getK8sPeers(k8sClient)
	if err != nil {
		return trace.Wrap(err, "failed to get k8s peers")
	}

	serfPeers, err := getSerfPeers(serfClient)
	if err != nil {
		return trace.Wrap(err, "failed to get serf peers")
	}

	if !shouldReconcileSerf(k8sPeers, serfPeers) {
		return nil
	}

	if _, err := serfClient.Join(k8sPeers, false); err != nil {
		return trace.Wrap(err, "failed to join nodes")
	}

	newPeers, err := getSerfPeers(serfClient)
	if err != nil {
		return trace.Wrap(err, "failed to get serf peers")
	}

	log.WithField("prev-cluster", serfPeers).
		WithField("new-cluster", newPeers).
		Info("Successfully reconciled serf cluster.")

	return nil
}

// getK8sPeers returns advertised IP addresses of all nodes in the kubernetes
// cluster.
func getK8sPeers(client kubernetes.Interface) (peers []string, err error) {
	nodes, err := client.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return peers, trace.Wrap(err, "failed to list kubernetes nodes")
	}

	for _, node := range nodes.Items {
		addr, exists := node.Labels[advertiseIPKey]
		if !exists {
			return peers, trace.NotFound("node %s does not have %s label", node.Name, advertiseIPKey)
		}
		peers = append(peers, addr)
	}
	return peers, nil
}

// getSerfPeers returns the advertised IP address of all nodes in the serf
// cluster.
func getSerfPeers(client serfClient) (peers []string, err error) {
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
		if _, exists := missing[serfPeer]; !exists {
			log.WithField("addr", serfPeer).
				Warn("Serf member is no longer a member of the gravity cluster and should be removed from the serf cluster.")
			continue
		}
		delete(missing, serfPeer)
	}
	return len(missing) != 0
}

// serfClient interface can be used to query the members of the cluster.
// This interface is defined so that a mock serf client implementation
// can be provided for unit tests.
type serfClient interface {
	// Members lists the members of the cluster.
	Members() ([]serf.Member, error)
	// Join joins the peers' clusters.
	Join(peers []string, replay bool) (int, error)
}

// advertiseIPKey specifies the key mapped to the advertised IP address.
const advertiseIPKey = "gravitational.io/advertise-ip"
