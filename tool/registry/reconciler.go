package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/utils"

	"github.com/gravitational/satellite/cmd"
	"github.com/gravitational/satellite/lib/ctxgroup"
	"github.com/gravitational/trace"

	"github.com/cenkalti/backoff"
	"github.com/jonboulle/clockwork"
	"github.com/sirupsen/logrus"
	coordinationv1 "k8s.io/api/coordination/v1"
	v1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1beta1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	coordinationclient "k8s.io/client-go/kubernetes/typed/coordination/v1"
	discoveryclient "k8s.io/client-go/kubernetes/typed/discovery/v1beta1"
	"k8s.io/utils/pointer"
)

func startEndpointsReconciler(addr string, g *ctxgroup.Group) error {
	client, err := cmd.GetKubeClientFromPath(constants.SchedulerConfigPath)
	if err != nil {
		return trace.Wrap(err)
	}

	rl := &leaseReconciler{
		FieldLogger: logrus.WithFields(logrus.Fields{
			trace.Component: "leases",
			"addr":          addr,
		}),
		addr:                 addr,
		leaseDurationSeconds: leaseTTLSeconds,
		clock:                clockwork.NewRealClock(),
		leaseClient:          client.CoordinationV1(),
	}

	r := &endpointReconciler{
		FieldLogger: logrus.WithFields(logrus.Fields{
			trace.Component: "endpoints",
			"addr":          addr,
		}),
		addr:                addr,
		clock:               clockwork.NewRealClock(),
		resyncInterval:      endpointResyncInterval,
		leaseClient:         client.CoordinationV1(),
		endpointSliceClient: client.DiscoveryV1beta1(),
	}

	g.GoCtx(rl.sync)
	g.GoCtx(r.sync)
	return nil
}

func (r *endpointReconciler) sync(ctx context.Context) error {
	ticker := r.clock.NewTicker(r.resyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.Chan():
			if err := r.reconcile(ctx); err != nil {
				r.WithError(err).Warn("Failed to reconcile.")
			}
		case <-ctx.Done():
			return trace.Wrap(ctx.Err())
		}
	}
}

func (r *endpointReconciler) reconcile(ctx context.Context) error {
	addrList, err := r.leaseClient.Leases(leaseNamespace).List(metav1.ListOptions{
		LabelSelector: leaseSelector.String(),
	})
	if err != nil {
		return trace.Wrap(err)
	}
	if len(addrList.Items) == 0 {
		// Do not remove the endpoints completely
		return trace.NotFound("no registry endpoints in storage")
	}
	var addrs []string
	for _, addr := range addrList.Items {
		if addr.Spec.HolderIdentity != nil {
			addrs = append(addrs, *addr.Spec.HolderIdentity)
		}
	}
	return r.updateEndpoints(addrs)
}

func (r *endpointReconciler) updateEndpoints(addrs []string) error {
	slice := newEndpointSlice(addrs)
	client := r.endpointSliceClient.EndpointSlices(metav1.NamespaceSystem)
	existingSlice, err := client.Get("registry", metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = client.Create(slice)
		return trace.Wrap(err)
	}
	if err != nil {
		return trace.Wrap(err)
	}
	if apiequality.Semantic.DeepEqual(slice.Endpoints, existingSlice.Endpoints) &&
		apiequality.Semantic.DeepEqual(slice.Ports, existingSlice.Ports) &&
		apiequality.Semantic.DeepEqual(slice.Labels, existingSlice.Labels) {
		// Nothing to do
		return nil
	}
	_, err = client.Update(slice)
	return trace.Wrap(err)
}

type endpointReconciler struct {
	logrus.FieldLogger
	addr                string
	resyncInterval      time.Duration
	clock               clockwork.Clock
	leaseClient         coordinationclient.LeasesGetter
	endpointSliceClient discoveryclient.EndpointSlicesGetter
}

func (r *leaseReconciler) sync(ctx context.Context) error {
	ticker := r.clock.NewTicker(time.Duration(r.leaseDurationSeconds) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.Chan():
			if _, err := r.reconcile(ctx); err != nil {
				r.WithError(err).Warn("Failed to reconcile lease.")
			}
		case <-ctx.Done():
			return trace.Wrap(ctx.Err())
		}
	}
}

func (r *leaseReconciler) reconcile(ctx context.Context) (lease *coordinationv1.Lease, err error) {
	b := backoff.NewExponentialBackOff()
	b.MaxInterval = 10 * time.Second
	b.MaxElapsedTime = 0 // unlimited
	err = utils.RetryWithInterval(ctx, b, func() error {
		lease, err = r.ensureLease()
		return trace.Wrap(err)
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return lease, nil
}

func (r *leaseReconciler) ensureLease() (*coordinationv1.Lease, error) {
	leaseClient := r.leaseClient.Leases(leaseNamespace)
	lease, err := leaseClient.Get(r.addr, metav1.GetOptions{})
	// TODO: need to differentiate between created/existing?
	if err == nil {
		return lease, nil
	}
	if apierrors.IsNotFound(err) {
		lease = r.newLease(nil)
		_, err = leaseClient.Create(lease)
	}
	return lease, trace.Wrap(err)
}

func (r *leaseReconciler) newLease(base *coordinationv1.Lease) (lease *coordinationv1.Lease) {
	if base == nil {
		lease = &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      leaseName(r.addr),
				Namespace: leaseNamespace,
				Labels:    leaseLabels,
			},
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity:       pointer.StringPtr(r.addr),
				LeaseDurationSeconds: pointer.Int32Ptr(r.leaseDurationSeconds),
			},
		}
	} else {
		lease = base.DeepCopy()
	}
	lease.Spec.RenewTime = &metav1.MicroTime{Time: r.clock.Now()}
	return lease
}

type leaseReconciler struct {
	logrus.FieldLogger
	addr                 string
	leaseDurationSeconds int32
	lease                *coordinationv1.Lease
	clock                clockwork.Clock
	leaseClient          coordinationclient.LeasesGetter
}

func newEndpointSlice(addrs []string) *discovery.EndpointSlice {
	ready := true
	proto := v1.ProtocolTCP
	port := int32(5000)
	var ports []discovery.EndpointPort
	for range addrs {
		ports = append(ports, discovery.EndpointPort{
			Protocol: &proto,
			Port:     &port,
		})
	}
	return &discovery.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "registry",
			Labels: leaseLabels,
		},
		AddressType: discovery.AddressTypeIPv4,
		Endpoints: []discovery.Endpoint{
			{
				Addresses: addrs,
				Conditions: discovery.EndpointConditions{
					Ready: &ready,
				},
			},
		},
		Ports: ports,
	}
}

func leaseName(addr string) string {
	return fmt.Sprintf("registry-%v", strings.ReplaceAll(addr, ".", "-"))
}

var (
	leaseLabels            = labels.Set{"gravitational.io/service": "registry"}
	leaseSelector          = labels.SelectorFromSet(leaseLabels)
	leaseNamespace         = metav1.NamespaceSystem
	endpointSliceNamespace = metav1.NamespaceSystem
)

const (
	// renewIntervalFraction is the fraction of lease duration to renew the lease
	renewIntervalFraction = 0.25

	leaseTTLSeconds        = 30
	endpointResyncInterval = 30 * time.Second
)
