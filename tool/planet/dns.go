package main

import (
	"context"
	"time"

	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/utils"
	"github.com/gravitational/satellite/agent"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/monitoring"
	"github.com/gravitational/trace"

	log "github.com/Sirupsen/logrus"
	kube "k8s.io/client-go/1.4/kubernetes"
	"k8s.io/client-go/1.4/pkg/api/v1"
	"k8s.io/client-go/1.4/pkg/util/intstr"
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
func (r *DNSBootstrapper) createService(client *kube.Clientset) (err error) {
	const service = "kube-dns"
	err = createServiceNamespaceIfNeeded(client)
	if err != nil {
		return trace.Wrap(err)
	}
	dnsService := &v1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name: service,
			Labels: map[string]string{
				"k8s-app":                       "kube-dns",
				"kubernetes.io/cluster-service": "true",
				"kubernetes.io/name":            "KubeDNS",
			},
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{
				"k8s-app": "kube-dns",
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
	const retryAttempts = 50
	var client *kube.Clientset
	var err error

	err = utils.Retry(context.TODO(), retryAttempts, retryPeriod, func() error {
		var status *pb.NodeStatus
		status = r.agent.LocalStatus()
		if status.Status != pb.NodeStatus_Running {
			return trace.ConnectionProblem(nil, "node unhealthy: %v retrying", status.Status)
		}

		// kube client is also a part of the retry loop as the kubernetes
		// API server might not be available at first connect
		if client == nil {
			client, err = monitoring.ConnectToKube(r.kubeAddr, constants.KubeConfigPath)
			if err != nil {
				return trace.ConnectionProblem(err, "failed to connect to kubernetes")
			}
		}
		err = r.createService(client)
		if err != nil {
			return trace.Wrap(err, "failed to create kube-dns service")
		}
		log.Infof("created kube-dns service")
		return nil
	})
	if err != nil {
		log.Errorf("giving up on creating kube-dns: %v", trace.DebugReport(err))
	}
}

// createServiceNamespaceIfNeeded creates a service namespace if one does not exist yet.
func createServiceNamespaceIfNeeded(client *kube.Clientset) error {
	log.Infof("creating %s namespace", serviceNamespace)
	if _, err := client.Namespaces().Get(serviceNamespace); err != nil {
		log.Infof("%s namespace not found: %v", serviceNamespace, err)
		_, err = client.Namespaces().Create(&v1.Namespace{ObjectMeta: v1.ObjectMeta{Name: serviceNamespace}})
		if err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

// upsertService either creates a new service if the specified service does not exist,
// or updates an existing one.
func upsertService(client *kube.Clientset, service *v1.Service) (err error) {
	log.Infof("creating %s service with spec:\n%#v", service.ObjectMeta.Name, service)
	serviceName := service.ObjectMeta.Name
	services := client.Services(serviceNamespace)
	var existing *v1.Service
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
