package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"path/filepath"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"

	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/utils"

	"github.com/gravitational/satellite/agent"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/monitoring"
	"github.com/gravitational/trace"

	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	api "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/watch"
	kube "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
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
			client, err = monitoring.ConnectToKube(r.kubeAddr, constants.SchedulerConfigPath)
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

type coreDNSMonitor struct {
	config     coreDNSConfig
	controller cache.Controller
	store      cache.Store
}

// NewCoreDNSMonitor updates local coreDNS configuration based on defaults or a config map present
// in k8s.
func runCoreDNSMonitor(ctx context.Context, config coreDNSConfig) error {
	log.Debug("runCoreDNSMonitor")
	client, err := monitoring.ConnectToKube(constants.KubeAPIEndpoint, constants.KubectlConfigPath)
	if err != nil {
		return trace.Wrap(err)
	}

	go coreDnsLoop(ctx, config, client)
	return nil
}

func coreDnsLoop(ctx context.Context, config coreDNSConfig, client *kube.Clientset) {
	log.Debug("coreDNSLoop")

	var overlayAddrs []string
	var err error

	ticker := time.NewTicker(5 * time.Second)
T:
	for {
		log.Debug("in timing loop")
		select {
		case <-ticker.C:
			log.Debug("trying to find overlay network address")
			overlayAddrs, err = getAddressesByInterface(constants.OverlayInterfaceName)

			if err != nil && !trace.IsNotFound(err) {
				log.Warnf("unexpected error attempting to find interface %v addresses: %v",
					constants.OverlayInterfaceName, trace.DebugReport(err))
			}

			if err != nil {
				log.Debug("error retrieving overlay address: ", trace.DebugReport(err))
				continue
			}

			log.Debug("retrieved overlay network addresses: ", overlayAddrs)

			line := fmt.Sprintf("%v=\"%v\"\n", EnvOverlayAddresses, strings.Join(overlayAddrs, ","))
			err = ioutil.WriteFile(OverlayEnvFile, []byte(line), 644)
			if err != nil {
				log.Warnf("Failed to write overlay environment %v: %v", OverlayEnvFile, err)
				continue
			}

			break T

		case <-ctx.Done():
			log.Debug("coredns context cancelled")
			return
		}
	}
	ticker.Stop()

	// replace the ListenAddrs with the overlay network address(es)
	// since this is replacing the cluster dns IP
	config.ListenAddrs = overlayAddrs
	monitor := coreDNSMonitor{
		config: config,
	}

	log.Debug("monitoring kube-system/coredns configmap")
	monitor.monitorConfigMap(ctx, client)

	// hold the goroutine until cancelled
	log.Debug("waiting for shutdown")
	<-ctx.Done()
}

func getAddressesByInterface(iface string) ([]string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	for _, i := range ifaces {
		if i.Name == iface {
			a, err := i.Addrs()
			if err != nil {
				return nil, trace.Wrap(err)
			}
			addrs := make([]string, len(a))
			for _, addr := range a {
				var ip net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}
				if ip == nil || ip.IsLoopback() {
					continue
				}
				if ip.To4() != nil {
					addrs = append(addrs, ip.String())
				}
			}
			return addrs, nil
		}
	}
	return nil, trace.NotFound("interface %v not found", iface)
}

func (c *coreDNSMonitor) monitorConfigMap(ctx context.Context, client *kube.Clientset) {
	c.store, c.controller = cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				options.FieldSelector = fields.OneTermEqualSelector(
					"metadata.name",
					constants.CoreDNSConfigMapName,
				).String()
				return client.CoreV1().ConfigMaps(metav1.NamespaceSystem).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				options.FieldSelector = fields.OneTermEqualSelector(
					"metadata.name",
					constants.CoreDNSConfigMapName,
				).String()
				return client.CoreV1().ConfigMaps(metav1.NamespaceSystem).Watch(options)
			},
		},
		&api.ConfigMap{},
		15*time.Minute,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.add,
			UpdateFunc: c.update,
			DeleteFunc: c.delete,
		},
	)
	c.controller.Run(ctx.Done())

}

func (c *coreDNSMonitor) add(obj interface{}) {
	log.Warn("coreDNSMonitor.add")
	c.processCoreDNSConfigChange(obj)
}

func (c *coreDNSMonitor) delete(obj interface{}) {
	log.Warn("coreDNSMonitor.delete")
	c.processCoreDNSConfigChange(nil)
}

func (c *coreDNSMonitor) update(oldObj, newObj interface{}) {
	log.Warn("coreDNSMonitor.update")
	c.processCoreDNSConfigChange(newObj)
}

func (c *coreDNSMonitor) processCoreDNSConfigChange(newObj interface{}) {
	log.Warn("processCoreDNSConfigChange: ", spew.Sdump(newObj))
	template := coreDNSTemplate

	// If we have a configuration we can use from a kubernetes configmap
	// use it as a template to generate our config, otherwise, generate
	// using the default template embedded
	if newObj != nil {
		configMap, ok := newObj.(*api.ConfigMap)
		if !ok {
			log.Errorf("recieved unexpected object callback: %T", newObj)
			return
		}

		t, ok := configMap.Data["Corefile"]
		if !ok {
			log.Warn("recieved configmap doesn't contain corefile key: ", spew.Sdump(configMap))
		} else {
			template = t
		}
	}

	config, err := generateCoreDNSConfig(c.config, template)
	if err != nil {
		log.Error("failed to template coredns configuration: ", err)
		return
	}

	err = ioutil.WriteFile(filepath.Join(CoreDNSClusterConf), []byte(config), SharedFileMask)
	if err != nil {
		log.Errorf("failed to write coredns configuration to %v: %v", CoreDNSClusterConf, err)
	}
}
