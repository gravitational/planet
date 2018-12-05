package main

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/utils"

	"github.com/davecgh/go-spew/spew"
	"github.com/gravitational/satellite/cmd"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
	api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	kube "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type coreDNSMonitor struct {
	config     coreDNSConfig
	controller cache.Controller
	store      cache.Store
}

// runCoreDNSMonitor updates local coreDNS configuration
// it will monitor k8s for a configmap, and use the configmap to generate the local coredns configuration
// if the configmap isn't present, it will generate the configuration based on defaults
func runCoreDNSMonitor(ctx context.Context, config coreDNSConfig) error {
	client, err := cmd.GetKubeClientFromPath(constants.CoreDNSConfigPath)
	if err != nil {
		return trace.Wrap(err)
	}

	go coreDNSLoop(ctx, config, client)
	return nil
}

func coreDNSLoop(ctx context.Context, config coreDNSConfig, client *kube.Clientset) {
	var overlayAddrs []string
	var err error

	// Try and retrieve the IP address assigned to our network interface in the overlay network
	// this may not be available when we start, so we loop forever in a background routine
	// until it becomes available
	ticker := time.NewTicker(5 * time.Second)
T:
	for {
		select {
		case <-ticker.C:
			overlayAddrs, err = getAddressesByInterface(constants.OverlayInterfaceName)
			if err != nil {
				if trace.IsNotFound(err) {
					continue
				}
				log.Warnf("Unexpected error attempting to find interface %v addresses: %v",
					constants.OverlayInterfaceName, trace.DebugReport(err))
			}

			line := fmt.Sprintf("%v=\"%v\"\n", EnvOverlayAddresses, strings.Join(overlayAddrs, ","))
			log.Debug("Creating overlay env: ", line)
			err = utils.SafeWriteFile(OverlayEnvFile, strings.NewReader(line), constants.SharedReadMask)
			if err != nil {
				log.Warnf("Failed to write overlay environment %v: %v", OverlayEnvFile, err)
				continue
			}

			break T

		case <-ctx.Done():
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

	// make sure we generate a default configuration during startup
	monitor.processCoreDNSConfigChange(nil)

	monitor.monitorConfigMap(ctx, client)
}

// getAddressesByInterface inspects the local network interfaces, and returns a list of
// IPv4 addresses assigned to the interface
func getAddressesByInterface(iface string) ([]string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	for _, i := range ifaces {
		if i.Name != iface {
			continue
		}
		a, err := i.Addrs()
		if err != nil {
			return nil, trace.Wrap(err)
		}
		addrs := make([]string, 0)
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
		if len(addrs) > 0 {
			return addrs, nil
		}
		return nil, trace.NotFound("no addresses found on %v", iface)

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
	c.processCoreDNSConfigChange(obj)
}

func (c *coreDNSMonitor) delete(obj interface{}) {
	c.processCoreDNSConfigChange(nil)
}

func (c *coreDNSMonitor) update(oldObj, newObj interface{}) {
	c.processCoreDNSConfigChange(newObj)
}

func (c *coreDNSMonitor) processCoreDNSConfigChange(newObj interface{}) {
	// If we have a configuration we can use from a kubernetes configmap
	// use it as a template to generate our config, otherwise, generate
	// using the default template embedded
	template := coreDNSTemplate
	if newObj != nil {
		configMap, ok := newObj.(*api.ConfigMap)
		if !ok {
			log.Errorf("Received unexpected object callback: %T", newObj)
			return
		}

		t, ok := configMap.Data["Corefile"]
		if !ok {
			log.Warn("Received configmap doesn't contain Corefile data: ", spew.Sdump(configMap))
		} else {
			template = t
		}
	}

	config, err := generateCoreDNSConfig(c.config, template)
	if err != nil {
		log.Error("Failed to template coredns configuration: ", err)
		return
	}

	err = utils.SafeWriteFile(filepath.Join(CoreDNSClusterConf), strings.NewReader(config), constants.SharedReadMask)
	if err != nil {
		log.Errorf("Failed to write coredns configuration to %v: %v", CoreDNSClusterConf, err)
	}

	err = exec.Command("killall", "-SIGUSR1", "coredns").Run()
	if err != nil {
		log.Errorf("Error sending SIGUSR1 to coredns: %v", err)
	}
}
