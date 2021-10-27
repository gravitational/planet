/*
Copyright 2018 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/ipallocator"
	"github.com/gravitational/planet/lib/utils"

	"github.com/cenkalti/backoff"
	"github.com/gravitational/satellite/agent"
	"github.com/gravitational/satellite/cmd"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

// setupResolver finds the kube-dns service address, and writes an environment file accordingly
func setupResolver(ctx context.Context, role agent.Role, serviceCIDR net.IPNet) error {
	client, err := cmd.GetKubeClientFromPath(constants.KubeletConfigPath)
	if err != nil {
		return trace.Wrap(err)
	}
	err = utils.RetryWithInterval(ctx, newUnlimitedExponentialBackoff(5*time.Second), func() error {
		err = updateEnvDNSAddresses(ctx, client, role, serviceCIDR)
		if err != nil {
			log.Warn("Error updating DNS env: ", err)
			return trace.Wrap(err)
		}
		return nil

	})
	return trace.Wrap(err)
}

func writeEnvDNSAddresses(addr []string, overwrite bool) error {
	env := fmt.Sprintf(`%v="%v"`, EnvDNSAddresses, strings.Join(addr, ","))
	env = fmt.Sprintln(env)

	if _, err := os.Stat(DNSEnvFile); !os.IsNotExist(err) && !overwrite {
		return nil
	}

	err := utils.SafeWriteFile(DNSEnvFile, []byte(env), constants.SharedReadMask)
	return trace.Wrap(err)
}

func updateEnvDNSAddresses(ctx context.Context, client *kubernetes.Clientset, role agent.Role, serviceCIDR net.IPNet) error {
	// locate the cluster IP of the kube-dns service
	masterServices, err := client.CoreV1().Services(metav1.NamespaceSystem).List(ctx, metav1.ListOptions{
		LabelSelector: dnsServiceSelector.String(),
	})
	if err != nil {
		return trace.Wrap(err)
	}
	svcMaster, err := getDNSService(masterServices.Items, serviceCIDR)
	if err != nil {
		return trace.Wrap(err)
	}

	workerServices, err := client.CoreV1().Services(metav1.NamespaceSystem).List(ctx, metav1.ListOptions{
		LabelSelector: dnsWorkerServiceSelector.String(),
	})
	if err != nil {
		return trace.Wrap(err)
	}
	svcWorker, err := getDNSService(workerServices.Items, serviceCIDR)
	if err != nil {
		return trace.Wrap(err)
	}

	// If we're a master server, only use the master servers as a resolver.
	// This is because, we don't know if the second worker service will have any pods after future scaling operations
	//
	// If we're a worker, query the workers coredns first, and master second
	// This guaranteess any retries will not be handled by the same node
	if role == agent.RoleMaster {
		return trace.Wrap(writeEnvDNSAddresses([]string{svcMaster.Spec.ClusterIP}, true))
	}
	return trace.Wrap(writeEnvDNSAddresses([]string{svcWorker.Spec.ClusterIP, svcMaster.Spec.ClusterIP}, true))
}

func ensureDNSServices(ctx context.Context, serviceCIDR net.IPNet) error {
	client, err := cmd.GetKubeClientFromPath(constants.SchedulerConfigPath)
	if err != nil {
		return trace.Wrap(err)
	}
	ipalloc := ipallocator.NewAllocatorCIDRRange(&serviceCIDR)
	services := client.CoreV1().Services(metav1.NamespaceSystem)
	for _, name := range []string{"kube-dns", "kube-dns-worker"} {
		if err := ensureDNSService(ctx, name, services, ipalloc); err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

// ensureDNSService creates the kubernetes DNS service if it doesn't already exist.
// The service object is managed by gravity, but we create a placeholder here, so that we can read the IP address
// of the service, and configure kubelet with the correct DNS addresses before starting
func ensureDNSService(ctx context.Context, name string, services corev1.ServiceInterface, ipalloc *ipallocator.Range) error {
	ip, err := ipalloc.AllocateNext()
	if err != nil && err != ipallocator.ErrFull {
		return trace.Wrap(err)
	}
	logger := log.WithField("dns-service", name)
	return utils.RetryWithInterval(ctx, newUnlimitedExponentialBackoff(5*time.Second), func() error {
		_, err = services.Create(ctx, newDNSService(name, ip.String()), metav1.CreateOptions{})
		if err == nil || errors.IsAlreadyExists(err) {
			logger.Info("Service exists.")
			return nil
		}
		if isIPAlreadyAllocatedError(err) {
			ipalloc.Release(ip)
			ip, err = ipalloc.AllocateNext()
			if err != nil {
				return &backoff.PermanentError{Err: err}
			}
		}
		logger.WithError(err).Warn("Error creating service.")
		return trace.Wrap(err)
	})
}

func newDNSService(name, clusterIP string) *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceSystem,
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
				},
			},
			SessionAffinity: "None",
			ClusterIP:       clusterIP,
		},
	}
}

func getDNSService(services []v1.Service, serviceCIDR net.IPNet) (*v1.Service, error) {
	for _, service := range services {
		logger := log.WithFields(log.Fields{
			"service":      fmt.Sprintf("%v/%v", service.Namespace, service.Name),
			"service-cidr": serviceCIDR.String(),
		})
		if service.Spec.ClusterIP == "" {
			logger.Warn("Service does not have ClusterIP - will skip.")
			continue
		}
		ipAddr := net.ParseIP(service.Spec.ClusterIP)
		if ipAddr == nil {
			logger.WithField("addr", service.Spec.ClusterIP).Warn("Invalid ClusterIP - will skip.")
			continue
		}
		if !serviceCIDR.Contains(ipAddr) {
			logger.WithField("cluster-ip", service.Spec.ClusterIP).Warn("Service has ClusterIP not from service CIDR.")
			continue
		}
		return &service, nil
	}
	return nil, trace.NotFound("no DNS service matched")
}

func newUnlimitedExponentialBackoff(maxInterval time.Duration) *backoff.ExponentialBackOff {
	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = 0
	b.MaxInterval = maxInterval
	return b
}

func mustLabelSelector(m map[string]string) labels.Selector {
	selector, err := metav1.LabelSelectorAsSelector(
		metav1.SetAsLabelSelector(m),
	)
	if err != nil {
		panic(err.Error())
	}
	return selector
}

// isIPAlreadyAllocatedError detects whether the given error indicates that the specified
// cluster IP is already allocated.
// This can happen since we are not syncing the IP allocation with the apiserver
func isIPAlreadyAllocatedError(err error) bool {
	switch err := err.(type) {
	case *errors.StatusError:
		return err.ErrStatus.Status == "Failure" && statusHasCause(err.ErrStatus,
			"spec.clusterIP", "provided IP is already allocated")
	}
	return false
}

func statusHasCause(status metav1.Status, field, messagePattern string) bool {
	if status.Details == nil {
		return false
	}
	for _, cause := range status.Details.Causes {
		if cause.Field == field && strings.Contains(cause.Message, messagePattern) {
			return true
		}
	}
	return false
}

// dnsServiceSelector defines label selector to query DNS service
var dnsServiceSelector = mustLabelSelector(map[string]string{
	"k8s-app": "kube-dns",
})

// dnsWorkerServiceSelector defines label selector to query DNS worker service
var dnsWorkerServiceSelector = mustLabelSelector(map[string]string{
	"k8s-app": "kube-dns-worker",
})
