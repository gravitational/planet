package main

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
	discovery "k8s.io/api/discovery/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// runAPIServerEndpointsWatcher runs a watcher for API server endpoints and updates the CoreDNS
// hosts file accordingly
func runAPIServerEndpointsWatcher(ctx context.Context, client clientset.Interface, resyncPeriod time.Duration) {
	informerFactory := informers.NewSharedInformerFactoryWithOptions(client, resyncPeriod,
		informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			options.LabelSelector = labels.SelectorFromSet(labels.Set{
				"kubernetes.io/service-name": "kubernetes",
			}).String()
		}))
	endpointSliceInformer := informerFactory.Discovery().V1beta1().EndpointSlices().Informer()
	w := &watcher{
		FieldLogger:  log.WithField(trace.Component, "apiserver"),
		informer:     endpointSliceInformer,
		listerSynced: endpointSliceInformer.HasSynced,
	}
	go w.waitForCacheSynced(ctx.Done())
	endpointSliceInformer.AddEventHandlerWithResyncPeriod(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    w.handleAddEndpointSlice,
			UpdateFunc: w.handleUpdateEndpointSlice,
			DeleteFunc: w.handleDeleteEndpointSlice,
		},
		resyncPeriod,
	)
	informerFactory.Start(ctx.Done())
	<-ctx.Done()
}

// waitForCacheSynced waits for the cache to sync
func (r *watcher) waitForCacheSynced(stopCh <-chan struct{}) {
	r.Info("Starting api server endpoint slice watcher.")

	if !cache.WaitForNamedCacheSync("endpoint slice config", stopCh, r.listerSynced) {
		return
	}

	var eps []*discovery.EndpointSlice
	for _, item := range r.informer.GetStore().List() {
		if ep, ok := item.(*discovery.EndpointSlice); ok {
			eps = append(eps, ep)
		}
	}
	r.updateEndpoints(eps)
}

func (r *watcher) handleAddEndpointSlice(obj interface{}) {
	endpointSlice, ok := obj.(*discovery.EndpointSlice)
	if !ok {
		r.Warnf("Unexpected object type: %T", obj)
		return
	}
	r.WithField("endpoints", endpointSlice.Endpoints).Info("EndpointSlice added.")
	r.updateEndpoints([]*discovery.EndpointSlice{endpointSlice})
}

func (r *watcher) handleUpdateEndpointSlice(oldObj, newObj interface{}) {
	endpointSlice, ok := newObj.(*discovery.EndpointSlice)
	if !ok {
		r.Warnf("Unexpected object type: %T", newObj)
		return
	}
	r.WithField("endpoints", endpointSlice.Endpoints).Info("EndpointSlice updated.")
	r.updateEndpoints([]*discovery.EndpointSlice{endpointSlice})
}

func (r *watcher) handleDeleteEndpointSlice(obj interface{}) {
	endpointSlice, ok := obj.(*discovery.EndpointSlice)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			r.Warnf("Unexpected object type: %T", obj)
			return
		}
		if endpointSlice, ok = tombstone.Obj.(*discovery.EndpointSlice); !ok {
			r.Warnf("Unexpected object type: %T", obj)
			return
		}
	}
	r.WithField("endpoints", endpointSlice.Endpoints).Info("EndpointSlice deleted.")
	r.removeEndpoints([]*discovery.EndpointSlice{endpointSlice})
}

func (r *watcher) updateEndpoints(endpointSlices []*discovery.EndpointSlice) {
	r.mu.Lock()
	addrs := r.copyAddrs()
	r.mu.Unlock()
	for _, slice := range endpointSlices {
		for _, ep := range slice.Endpoints {
			if ep.Conditions.Ready == nil || !*ep.Conditions.Ready {
				continue
			}
			for _, addr := range ep.Addresses {
				addrs[addr] = struct{}{}
			}
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !reflect.DeepEqual(r.addrs, addrs) {
		r.addrs = addrs
		r.updateEndpointsLocked()
	}
}

func (r *watcher) removeEndpoints(endpointSlices []*discovery.EndpointSlice) {
	r.mu.Lock()
	addrs := r.copyAddrs()
	defer r.mu.Unlock()
	for _, slice := range endpointSlices {
		for _, ep := range slice.Endpoints {
			for _, addr := range ep.Addresses {
				delete(addrs, addr)
			}
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !reflect.DeepEqual(r.addrs, addrs) {
		r.addrs = addrs
		r.updateEndpointsLocked()
	}
}

func (r *watcher) updateEndpointsLocked() {
	var addrs []string
	for addr := range r.addrs {
		addrs = append(addrs, addr)
	}
	r.WithField("addrs", addrs).Info("Update server hosts.")
	updateDNS(addrs)
}

// copyAddrs returns a copy of the address cache.
// Must be called with r.mu held
func (r *watcher) copyAddrs() map[string]struct{} {
	addrs := make(map[string]struct{}, len(r.addrs))
	for addr := range r.addrs {
		addrs[addr] = struct{}{}
	}
	return addrs
}

type watcher struct {
	log.FieldLogger
	informer     cache.SharedIndexInformer
	listerSynced cache.InformerSynced
	mu           sync.Mutex
	// addrs is the address cache
	addrs map[string]struct{}
}
