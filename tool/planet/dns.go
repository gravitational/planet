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
	"math"
	"net"
	"os"
	"strings"
	"time"

	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/utils"

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
)

// setupResolver finds the kube-dns service address, and writes an environment file accordingly
func setupResolver(ctx context.Context, role agent.Role, serviceCIDR net.IPNet) error {
	client, err := cmd.GetKubeClientFromPath(constants.KubeletConfigPath)
	if err != nil {
		return trace.Wrap(err)
	}

	err = utils.Retry(ctx, math.MaxInt64, 1*time.Second, func() error {
		if role == agent.RoleMaster {
			for _, name := range []string{"kube-dns", "kube-dns-worker"} {
				err := createService(name)
				if err != nil {
					log.Warnf("Error creating service %v: %v.", name, err)
					return trace.Wrap(err)
				}
			}
		}

		err = updateEnvDNSAddresses(client, role, serviceCIDR)
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

func updateEnvDNSAddresses(client *kubernetes.Clientset, role agent.Role, serviceCIDR net.IPNet) error {
	// locate the cluster IP of the kube-dns service
	masterServices, err := client.CoreV1().Services(metav1.NamespaceSystem).List(metav1.ListOptions{
		LabelSelector: dnsServiceSelector.String(),
	})
	if err != nil {
		return trace.Wrap(err)
	}
	svcMaster, err := getDNSService(masterServices.Items, serviceCIDR)
	if err != nil {
		return trace.Wrap(err)
	}

	workerServices, err := client.CoreV1().Services(metav1.NamespaceSystem).List(metav1.ListOptions{
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

// createService creates the kubernetes DNS service if it doesn't already exist.
// The service object is managed by gravity, but we create a placeholder here, so that we can read the IP address
// of the service, and configure kubelet with the correct DNS addresses before starting
func createService(name string) error {
	client, err := cmd.GetKubeClientFromPath(constants.SchedulerConfigPath)
	if err != nil {
		return trace.Wrap(err)
	}
	service := &v1.Service{
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
				}},
			SessionAffinity: "None",
		},
	}
	_, err = client.CoreV1().Services(metav1.NamespaceSystem).Create(service)
	if err != nil && !errors.IsAlreadyExists(err) {
		return trace.Wrap(err)
	}
	return nil
}

func getDNSService(services []v1.Service, serviceCIDR net.IPNet) (*v1.Service, error) {
	for _, service := range services {
		logger := log.WithFields(log.Fields{
			"service":      fmt.Sprintf("%v/%v", service.Name, service.Namespace),
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
			logger.Warn("Service has ClusterIP not from service CIDR.")
			continue
		}
		return &service, nil
	}
	return nil, trace.NotFound("no DNS service matched")
}

func mustLabelSelector(labels ...string) labels.Selector {
	if len(labels)%2 != 0 {
		panic("must have even number of labels")
	}
	m := make(map[string]string)
	for i := 0; i < len(labels); i += 2 {
		m[labels[i]] = labels[i+1]
	}
	selector, err := metav1.LabelSelectorAsSelector(
		&metav1.LabelSelector{
			MatchLabels: map[string]string{
				"k8s-app": "kube-dns",
			},
		},
	)
	if err != nil {
		panic(err.Error())
	}
	return selector
}

// dnsServiceSelector defines label selector to query DNS service
var dnsServiceSelector = mustLabelSelector("k8s-app", "kube-dns")

// dnsWorkerServiceSelector defines label selector to query DNS worker service
var dnsWorkerServiceSelector = mustLabelSelector("k8s-app", "kube-dns-worker")
