package main

import (
	"context"
	"fmt"
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
	controller cache.Controller
	store      cache.Store
	client     *kube.Clientset
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
	monitor := coreDNSMonitor{}

	// make sure we generate a default configuration during startup
	monitor.processPodChange(nil)

	monitor.monitorDNSPod(ctx, client)
}

func (c *coreDNSMonitor) monitorDNSPod(ctx context.Context, client *kube.Clientset) {
	c.store, c.controller = cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				options.FieldSelector = fields.OneTermEqualSelector("spec.nodeName", os.Getenv(EnvNodeName)).String()
				options.LabelSelector = labels.Set{
					"k8s-app": "kube-dns",
				}.AsSelector().String()

				return client.CoreV1().Pods(metav1.NamespaceSystem).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				options.FieldSelector = fields.OneTermEqualSelector("spec.nodeName", os.Getenv(EnvNodeName)).String()
				options.LabelSelector = labels.Set{
					"k8s-app": "kube-dns",
				}.AsSelector().String()

				return client.CoreV1().Pods(metav1.NamespaceSystem).Watch(options)
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

	pod, ok := newObj.(*api.Pod)
	if !ok {
		log.Errorf("Received unexpected object callback: %T", newObj)
		return
	}

	var resolverIPs []string
	if newObj != nil {
		resolverIPs = append(resolverIPs, pod.Status.PodIP)
	}

	// try and locate the kube-dns svc clusterIP
	svc, err := c.client.CoreV1().Services(metav1.NamespaceSystem).Get("kube-dns", metav1.GetOptions{})
	if err != nil {
		log.Error("failed to retrieve kube-dns service: ", err)
	} else {
		resolverIPs = append(resolverIPs, svc.Spec.ClusterIP)
	}

	line := fmt.Sprintf("%v=\"%v\"\n", EnvDNSAddresses, strings.Join(resolverIPs, ","))
	log.Debug("Creating dns env: ", line)
	err = utils.SafeWriteFile(DNSEnvFile, []byte(line), constants.SharedReadMask)
	if err != nil {
		log.Warnf("Failed to write overlay environment %v: %v", DNSEnvFile, err)
		return
	}

	err = exec.Command("systemctl", "restart", "kube-kubelet").Run()
	if err != nil {
		log.Errorf("Error restart kubelet: %v", err)
	}
}
