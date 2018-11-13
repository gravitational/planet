package main

import (
	"context"
	"fmt"
	"math"
	"net"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/utils"
	"github.com/gravitational/satellite/cmd"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
	api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const kubeDNS = "kube-dns"

type corednsMonitor struct {
	config     corednsConfig
	controller cache.Controller
	store      cache.Store
}

// setupCoredns attempts to figure out what IP address to configure as the DNS resolver to use
// We support two modes of operation. Under flannel/wormhole, we use a local bridge, and want to bind coredns
// to the bridge, and have kubelet use the local coredns instance to resolve addresses.
//
// However, when using a third party network plugin, we can't guarantee it will use a bridge, or that we'll
// be able to locate the correct local interface to use. In this scenario, we use a more traditional k8s service
// as the DNS resolver address.
//
// This function tries to figure out which mode we're in, and configure ourselves accordingly
func setupCoredns(ctx context.Context, config corednsConfig) error {
	client, err := cmd.GetKubeClientFromPath(constants.KubeletConfigPath)
	if err != nil {
		return trace.Wrap(err)
	}

	resolverIP := ""

	log.Info("Attempting to determine dns resolver address")
	utils.Retry(ctx, math.MaxInt32, 1*time.Second, func() error {
		clusterIP, err := getkubeDNSServiceIP(ctx, client)
		if err != nil {
			// This could be a normal error, as we only create the kube-dns service
			log.Warn("Unable to retrieve kube-dns service: ", trace.DebugReport(err))
		}
		if clusterIP != "" {
			resolverIP = clusterIP
			return nil
		}

		for _, iface := range []string{constants.OverlayInterfaceName} {
			addr, err := getAddressesByInterface(iface)
			if err != nil {
				if !trace.IsNotFound(err) {
					// This could be a normal error, as it would be normal for the interface to not exist
					log.Warnf("Unable to locate bridge interface %v: %v.", iface, trace.DebugReport(err))
				}
			}
			if len(addr) > 0 {
				resolverIP = strings.Join(addr, ",")
				return nil
			}
		}

		return trace.NotFound("resolver IP address not found")
	})

	// write the resolver IP to an env file that kubelet will configure itself with
	line := fmt.Sprintf("%v=\"%v\"\n", EnvDNSAddresses, resolverIP)
	log.Debug("Creating overlay env: ", line)
	err = utils.SafeWriteFile(DNSEnvFile, []byte(line), constants.SharedReadMask)
	if err != nil {
		return trace.Wrap(err)
	}

	monitor := corednsMonitor{
		config: config,
	}

	// make sure we generate a default configuration during startup
	monitor.processCorednsConfigChange(nil)

	go monitor.monitorConfigMap(ctx)

	return nil
}

func getkubeDNSServiceIP(ctx context.Context, client *kubernetes.Clientset) (string, error) {
	log.Info("Retrieving service %v/%v spec.ClusterIP", metav1.NamespaceSystem, kubeDNS)

	svc, err := client.CoreV1().Services(metav1.NamespaceSystem).Get(kubeDNS, metav1.GetOptions{})
	if err != nil {
		return "", trace.Wrap(err)
	}
	return svc.Spec.ClusterIP, nil
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

func (c *corednsMonitor) monitorConfigMap(ctx context.Context) error {
	client, err := cmd.GetKubeClientFromPath(constants.CorednsConfigPath)
	if err != nil {
		log.Error("Unable to get coredns kubernetes client: ", err)
	}

	c.store, c.controller = cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				options.FieldSelector = fields.OneTermEqualSelector(
					"metadata.name",
					constants.CorednsConfigMapName,
				).String()
				return client.CoreV1().ConfigMaps(metav1.NamespaceSystem).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				options.FieldSelector = fields.OneTermEqualSelector(
					"metadata.name",
					constants.CorednsConfigMapName,
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
	return nil
}

func (c *corednsMonitor) add(obj interface{}) {
	c.processCorednsConfigChange(obj)
}

func (c *corednsMonitor) delete(obj interface{}) {
	c.processCorednsConfigChange(nil)
}

func (c *corednsMonitor) update(oldObj, newObj interface{}) {
	c.processCorednsConfigChange(newObj)
}

func (c *corednsMonitor) processCorednsConfigChange(newObj interface{}) {
	// If we have a configuration we can use from a kubernetes configmap
	// use it as a template to generate our config, otherwise, generate
	// using the default template embedded
	template := corednsTemplate
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

	config, err := generateCorednsConfig(c.config, template)
	if err != nil {
		log.Error("Failed to template coredns configuration: ", err)
		return
	}

	err = utils.SafeWriteFile(filepath.Join(CoreDNSClusterConf), []byte(config), SharedFileMask)
	if err != nil {
		log.Errorf("Failed to write coredns configuration to %v: %v", CoreDNSClusterConf, err)
	}

	err = exec.Command("killall", "-SIGUSR1", "coredns").Run()
	if err != nil {
		log.Errorf("Error sending SIGUSR1 to coredns: %v", err)
	}
}
