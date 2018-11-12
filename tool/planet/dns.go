package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/labels"

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
	controller   cache.Controller
	store        cache.Store
	client       *kube.Clientset
	clusterDNSIP string
}

// runCoreDNSMonitor updates local coreDNS configuration
// it will monitor k8s for a configmap, and use the configmap to generate the local coredns configuration
// if the configmap isn't present, it will generate the configuration based on defaults
func runCoreDNSMonitor(ctx context.Context, config coreDNSConfig) error {
	client, err := cmd.GetKubeClientFromPath(constants.KubeletConfigPath)
	if err != nil {
		return trace.Wrap(err)
	}

	monitor := coreDNSMonitor{
		client: client,
	}
	err = monitor.waitForDNSService(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	monitor.processPodChange(nil)
	go monitor.monitorDNSPod(ctx, client)

	return nil
}

func (c *coreDNSMonitor) waitForDNSService(ctx context.Context) error {
	log.Info("Looking for kube-dns service IP address")
	err := utils.Retry(ctx, math.MaxInt64, 1*time.Second, func() error {
		// try and locate the kube-dns svc clusterIP
		svc, err := c.client.CoreV1().Services(metav1.NamespaceSystem).Get("kube-dns", metav1.GetOptions{})
		if err != nil {
			log.Error("failed to retrieve kube-dns service: ", err)
			return trace.Wrap(err)
		}
		c.clusterDNSIP = svc.Spec.ClusterIP
		return nil
	})
	return trace.Wrap(err)
}

func (c *coreDNSMonitor) monitorDNSPod(ctx context.Context) {
	c.store, c.controller = cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				options.FieldSelector = fields.OneTermEqualSelector("spec.nodeName", os.Getenv(EnvNodeName)).String()
				options.LabelSelector = labels.Set{
					"k8s-app": "kube-dns",
				}.AsSelector().String()

				return c.client.CoreV1().Pods(metav1.NamespaceSystem).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				options.FieldSelector = fields.OneTermEqualSelector("spec.nodeName", os.Getenv(EnvNodeName)).String()
				options.LabelSelector = labels.Set{
					"k8s-app": "kube-dns",
				}.AsSelector().String()

				return c.client.CoreV1().Pods(metav1.NamespaceSystem).Watch(options)
			},
		},
		&api.Pod{},
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
	c.processPodChange(obj)
}

func (c *coreDNSMonitor) delete(obj interface{}) {
	c.processPodChange(nil)
}

func (c *coreDNSMonitor) update(oldObj, newObj interface{}) {
	c.processPodChange(newObj)
}

func (c *coreDNSMonitor) processPodChange(newObj interface{}) {
	log.Info("processPodChange: ", spew.Sdump(newObj))
	var resolverIPs []string

	if newObj != nil {
		pod, ok := newObj.(*api.Pod)
		if !ok {
			log.Errorf("Received unexpected object callback: %T", newObj)
			return
		}

		if newObj != nil {
			resolverIPs = append(resolverIPs, pod.Status.PodIP)
		}
	}

	// make sure, the cluster DNS is always the second in the list
	resolverIPs = append(resolverIPs, c.clusterDNSIP)

	line := fmt.Sprintf("%v=\"%v\"\n", EnvDNSAddresses, strings.Join(resolverIPs, ","))
	log.Debug("Creating dns env: ", line)
	err := utils.SafeWriteFile(DNSEnvFile, []byte(line), constants.SharedReadMask)
	if err != nil {
		log.Warnf("Failed to write overlay environment %v: %v", DNSEnvFile, err)
		return
	}

	// restart kubelet, so it picks up the new DNS settings
	err = exec.Command("systemctl", "restart", "kube-kubelet").Run()
	if err != nil {
		log.Errorf("Error restart kubelet: %v", err)
	}
}
