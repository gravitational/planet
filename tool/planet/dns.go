package main

import (
	"time"

	"github.com/gravitational/satellite/agent"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/monitoring"
	"github.com/gravitational/trace"

	log "github.com/Sirupsen/logrus"
	"k8s.io/kubernetes/pkg/api"
	kube "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/util/wait"
)

const serviceNamespace = "kube-system"

// DNSBootstrapper represents the process of creating a kubernetes service for DNS.
type DNSBootstrapper struct {
	clusterIP string
	kubeAddr  string
	agent     agent.Agent
}

// createKubeDNSService creates or updates the `kube-dns` kubernetes service.
// It will set the service's cluster IP to the value specified by clusterIP.
func (r *DNSBootstrapper) createService(client *kube.Client) (err error) {
	const service = "kube-dns"
	err = createServiceNamespaceIfNeeded(client)
	if err != nil {
		return trace.Wrap(err)
	}
	dnsService := &api.Service{
		ObjectMeta: api.ObjectMeta{
			Name: service,
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
			ClusterIP: r.clusterIP,
			Ports: []api.ServicePort{
				{
					Port:     3053,
					Protocol: "UDP",
					Name:     "dns",
				}, {
					Port:     3053,
					Protocol: "TCP",
					Name:     "dns-tcp",
				}},
			SessionAffinity: "None",
		},
	}
	err = upsertService(client, dnsService)
	if err != nil {
		return trace.Wrap(err, "failed to create DNS service")
	}
	return nil
}

// create runs a loop to creates/update the `kube-dns` kubernetes service.
// The loop continues until the master node has become healthy and the service
// gets created or a specified number of attempts have been made.
func (r *DNSBootstrapper) create() {
	const retryPeriod = 5 * time.Second
	const retryTimeout = 240 * time.Second
	var client *kube.Client

	if err := wait.Poll(retryPeriod, retryTimeout, func() (done bool, err error) {
		var status *pb.NodeStatus
		status = r.agent.LocalStatus()
		if status.Status != pb.NodeStatus_Running {
			log.Infof("node unhealthy, retrying")
			return false, nil
		}

		// kube client is also a part of the retry loop as the kubernetes
		// API server might not be available at first connect
		if client == nil {
			client, err = monitoring.ConnectToKube(r.kubeAddr)
			if err != nil {
				log.Infof("failed to connect to kubernetes: %v", err)
				return false, nil
			}
		}
		err = r.createService(client)
		if err != nil {
			log.Infof("failed to create kube-dns service: %v", err)
			return false, nil
		}
		log.Infof("created kube-dns service")
		return true, nil
	}); err != nil {
		log.Infof("giving up on creating kube-dns: %v", err)
	}
}

// createServiceNamespaceIfNeeded creates a service namespace if one does not exist yet.
func createServiceNamespaceIfNeeded(client *kube.Client) error {
	log.Infof("creating %s namespace", serviceNamespace)
	if _, err := client.Namespaces().Get(serviceNamespace); err != nil {
		log.Infof("%s namespace not found: %v", serviceNamespace, err)
		_, err = client.Namespaces().Create(&api.Namespace{ObjectMeta: api.ObjectMeta{Name: serviceNamespace}})
		if err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

// upsertService either creates a new service if the specified service does not exist,
// or updates an existing one.
func upsertService(client *kube.Client, service *api.Service) (err error) {
	log.Infof("creating %s service", service.ObjectMeta.Name)
	serviceName := service.ObjectMeta.Name
	services := client.Services(serviceNamespace)
	var existing *api.Service
	if existing, err = services.Get(serviceName); err != nil {
		log.Infof("%s service not found: %v", service.ObjectMeta.Name, err)
		_, err = services.Create(service)
		if err != nil {
			return trace.Wrap(err)
		}
		return nil
	}
	log.Infof("updating existing service %s: %v", existing.ObjectMeta.Name, existing)
	// FIXME: w/o the resource version reset, the etcd update fails with an error
	service.ObjectMeta.ResourceVersion = existing.ObjectMeta.ResourceVersion
	if _, err = services.Update(service); err != nil {
		return trace.Wrap(err)
	}
	return nil
}
