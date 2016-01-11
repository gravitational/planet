package main

import (
	"sync"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/planet/Godeps/_workspace/src/k8s.io/kubernetes/pkg/api"
	kube "github.com/gravitational/planet/Godeps/_workspace/src/k8s.io/kubernetes/pkg/client/unversioned"
	"github.com/gravitational/planet/Godeps/_workspace/src/k8s.io/kubernetes/pkg/util"
	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
	"github.com/gravitational/planet/lib/monitoring"
)

// This file implements process of bootstrapping kubernetes services required for DNS.

const serviceNamespace = "kube-system"

func bootstrapDNS(masterIP, clusterIP string, client *kube.Client) error {
	_, err := client.Namespaces().Create(&api.Namespace{ObjectMeta: api.ObjectMeta{Name: serviceNamespace}})
	if err != nil {
		// TODO: ensure this is not a `service exists` error (which should be ignored)
		return trace.Wrap(err)
	}
	etcdService := &api.Service{
		ObjectMeta: api.ObjectMeta{Name: "etcd"},
		Spec: api.ServiceSpec{
			// No selector - external service
			Ports: []api.ServicePort{{
				Port:       4001,
				TargetPort: util.NewIntOrStringFromInt(4001),
				Protocol:   "TCP",
				Name:       "client",
			}},
			SessionAffinity: "None",
		},
	}
	etcdService, err = client.Services(serviceNamespace).Create(etcdService)
	if err != nil {
		return trace.Wrap(err, "failed to create etcd service")
	}
	etcdEndpoints := &api.Endpoints{
		ObjectMeta: api.ObjectMeta{Name: etcdService.Name, Namespace: etcdService.Namespace},
		Subsets: []api.EndpointSubset{{
			Addresses: []api.EndpointAddress{{IP: masterIP}},
			Ports: []api.EndpointPort{{
				Name: etcdService.Spec.Ports[0].Name,
				Port: etcdService.Spec.Ports[0].Port,
			}},
		}},
	}
	etcdEndpoints, err = client.Endpoints(serviceNamespace).Create(etcdEndpoints)
	if err != nil {
		return trace.Wrap(err, "failed to create etcd service endpoints")
	}
	dnsService := &api.Service{
		ObjectMeta: api.ObjectMeta{
			Name: "kube-dns",
			Labels: map[string]string{
				"k8s-app":                       "kube-dns",
				"kubernetes.io/cluster-service": "true",
				"kubernetes.io/name":            "KubeDNS",
			},
		},
		Spec: api.ServiceSpec{
			Selector: map[string]string{
				"k8s-app": "kube-dns",
			},
			ClusterIP: clusterIP,
			Ports: []api.ServicePort{
				{
					Port:     53,
					Protocol: "UDP",
					Name:     "dns",
				}, {
					Port:     53,
					Protocol: "TCP",
					Name:     "dns-tcp",
				}},
			SessionAffinity: "None",
		},
	}
	dnsService, err = client.Services(serviceNamespace).Create(dnsService)
	if err != nil {
		return trace.Wrap(err, "failed to create dns service")
	}
	return nil
}

// dnsBootstrapper bootstraps a set of Kubernetes services for DNS.
// It subscribes to receive health check alerts and will configure the services
// once it has received the first positive result.
type dnsBootstrapper struct {
	sync.Once
	masterIP  string
	clusterIP string
	kubeAddr  string
}

// TODO: connect to planet agent and subscribe to health alerts.
// Might need to do multiple retries since it will start in parallel
// with the agent process and will have to make sure the agent has
// started before setting up health wait.

func (r *dnsBootstrapper) OnHealthCheck(status *pb.SystemStatus) {
	if status.Status == pb.SystemStatusType_SystemRunning {
		r.Do(func() {
			// TODO: retry on failure
			client, err := monitoring.ConnectToKube(r.kubeAddr)
			if err != nil {
				log.Errorf("dns: failed to connect to kubernetes: %v", err)
			} else {
				bootstrapDNS(r.masterIP, r.clusterIP, client)
			}
		})
	}
}
