package main

import (
	"context"
	"os"
	"os/exec"
	"time"

	"github.com/gravitational/planet/lib/monitoring"
	"github.com/gravitational/planet/lib/utils"
	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"

	"github.com/davecgh/go-spew/spew"
	api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

type cloudConfigMonitor struct {
	controller    cache.Controller
	store         cache.Store
	currentConfig string
}

func (c *cloudConfigMonitor) monitorConfigMap(ctx context.Context) {
	// only sync cloud-config if we're running with a loud provider
	cloudProvider := os.Getenv(EnvCloudProvider)
	if cloudProvider == "" {
		return
	}

	client, err := monitoring.GetPrivilegedKubeClient()
	if err != nil {
		logrus.WithError(err).Error("Error loading kubernetes client")
		return
	}

	c.store, c.controller = cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				options.FieldSelector = fields.OneTermEqualSelector(
					"metadata.name",
					CloudConfigMapName,
				).String()
				return client.CoreV1().ConfigMaps(metav1.NamespaceSystem).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				options.FieldSelector = fields.OneTermEqualSelector(
					"metadata.name",
					CloudConfigMapName,
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
	go c.controller.Run(ctx.Done())
	logrus.Infof("monitoring configmap %v/%v for updates", metav1.NamespaceSystem, CloudConfigMapName)
}

func (c *cloudConfigMonitor) add(obj interface{}) {
	c.processUpdate(obj)
}

func (c *cloudConfigMonitor) delete(obj interface{}) {
	c.processUpdate(nil)
}

func (c *cloudConfigMonitor) update(oldObj, newObj interface{}) {
	c.processUpdate(newObj)
}

func (c *cloudConfigMonitor) processUpdate(newObj interface{}) {

	cloudProvider := os.Getenv(EnvCloudProvider)
	clusterID := os.Getenv(EnvClusterID)
	gceNodeTags := os.Getenv(EnvGCENodeTags)

	// generate default configuration
	config, err := generateCloudConfig(cloudProvider, clusterID, gceNodeTags)
	if err != nil {
		logrus.Error("Failed to generate cloud-config.conf: ", trace.DebugReport(err))
	}

	if newObj != nil {
		configMap, ok := newObj.(*api.ConfigMap)
		if !ok {
			logrus.Errorf("Received unexpected object callback: %T", newObj)
			return
		}

		config, ok = configMap.Data["cloud-config.conf"]
		if !ok {
			logrus.Warn("Received configmap doesn't contain cloud-config.conf data: ", spew.Sdump(configMap))
			return
		}
	}
	// suppress writes if the config hasn't changed.
	// this is mainly so that we don't issue restarts to the controller manager as the object is refreshed
	if c.currentConfig == config {
		return
	}

	err = utils.SafeWriteFile(CloudConfigFile, []byte(config), SharedFileMask)
	if err != nil {
		logrus.Errorf("Failed to write %v: %v", CloudConfigFile, err)
	}

	err = exec.Command("killall", "-SIGTERM", "kube-controller-manager").Run()
	if err != nil {
		logrus.Errorf("Error sending SIGTERM to kube-controller-manager: %v", err)
	}

	// save the config
	c.currentConfig = config
	logrus.Infof("%v updated: %v", CloudConfigFile, config)
}
