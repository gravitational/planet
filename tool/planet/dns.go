package main

import (
	"context"
	"encoding/json"

	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/monitoring"
	"github.com/gravitational/planet/lib/utils"

	"github.com/gravitational/satellite/agent"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	kube "k8s.io/client-go/kubernetes"
)

const serviceNamespace = "kube-system"

// DNSBootstrapper represents the process of creating a kubernetes service for DNS.
type DNSBootstrapper struct {
	clusterIP           string
	upstreamNameservers []string
	dnsZones            map[string][]string
	kubeAddr            string
	agent               agent.Agent
}

// createLoop runs a loop to create/update the `kube-dns` kubernetes service.
// The loop continues until the master node has become healthy and the service
// has been created
func (r *DNSBootstrapper) createLoop() {
	var client *kube.Clientset

	err := utils.RetryWithInterval(context.TODO(), utils.NewUnlimitedExponentialBackOff(), func() (err error) {
		var status *pb.NodeStatus
		status = r.agent.LocalStatus()
		if status.Status != pb.NodeStatus_Running {
			return trace.ConnectionProblem(nil, "node unhealthy: %v retrying", status.Status)
		}
		// kube client is also a part of the retry loop as the kubernetes
		// API server might not be available at first connect
		if client == nil {
			client, err = monitoring.GetPrivilegedKubeClient()
			if err != nil {
				return trace.ConnectionProblem(err, "failed to connect to kubernetes")
			}
		}

		err = r.createService(client, metav1.NamespaceSystem, constants.DNSResourceName)
		if err != nil {
			return trace.Wrap(err, "failed to create kubedns service")
		}
		log.Info("Created kubedns service.")

		err = r.createConfigmap(client, metav1.NamespaceSystem, constants.DNSResourceName)
		if err != nil {
			return trace.Wrap(err, "failed to create kubedns configuration")
		}
		log.Info("Created kubedns configuration.")

		return nil
	})
	if err != nil {
		log.Errorf("Giving up on creating kubedns service: %v.", trace.DebugReport(err))
	}
}

// createService creates or updates the kubernetes DNS service.
// It will set the service's cluster IP to the value specified by clusterIP.
func (r *DNSBootstrapper) createService(client *kube.Clientset, namespace, name string) (err error) {
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"k8s-app":                       name,
				"kubernetes.io/cluster-service": "true",
				"kubernetes.io/name":            "KubeDNS",
			},
			ResourceVersion: "0",
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{
				"k8s-app": name,
			},
			ClusterIP: r.clusterIP,
			Ports: []v1.ServicePort{
				{
					Port:       53,
					TargetPort: intstr.FromString("dns"),
					Protocol:   "UDP",
					Name:       "dns",
				}, {
					Port:       53,
					Protocol:   "TCP",
					Name:       "dns-tcp",
					TargetPort: intstr.FromString("dns-tcp"),
				}},
			SessionAffinity: "None",
		},
	}

	_, err = client.CoreV1().Services(metav1.NamespaceSystem).Get(service.Name, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return trace.Wrap(err, "failed to query kubedns service")
		}

		if _, err := client.CoreV1().Services(metav1.NamespaceSystem).Create(service); err != nil {
			return trace.Wrap(err, "failed to create kubedns service")
		}
		return nil
	}
	if _, err = client.CoreV1().Services(metav1.NamespaceSystem).Update(service); err != nil {
		return trace.Wrap(err, "failed to update kubedns service")
	}
	return nil
}

func (r *DNSBootstrapper) createConfigmap(client *kube.Clientset, namespace, name string) (err error) {
	nameserversJSON, err := json.Marshal(r.upstreamNameservers)
	if err != nil {
		return trace.Wrap(err)
	}
	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string]string{
			"upstreamNameservers": string(nameserversJSON),
		},
	}

	if len(r.dnsZones) > 0 {
		stubDomainsJSON, err := json.Marshal(r.dnsZones)
		if err != nil {
			return trace.Wrap(err)
		}
		configMap.Data["stubDomains"] = string(stubDomainsJSON)
	}

	if _, err := client.CoreV1().ConfigMaps(metav1.NamespaceSystem).Create(configMap); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return trace.Wrap(err, "failed to create kubedns configmap")
		}

		if _, err := client.CoreV1().ConfigMaps(metav1.NamespaceSystem).Update(configMap); err != nil {
			return trace.Wrap(err, "failed to update kubedns configmap")
		}
	}
	return nil
}
