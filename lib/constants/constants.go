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

package constants

const (
	// KubectlConfigPath is the path to kubectl configuration file
	KubectlConfigPath = "/etc/kubernetes/kubectl.kubeconfig"
	// SchedulerConfigPath is the path to kube-scheduler configuration file
	SchedulerConfigPath = "/etc/kubernetes/scheduler.kubeconfig"
	// ProxyConfigPath is the path to kube-proxy configuration file
	ProxyConfigPath = "/etc/kubernetes/proxy.kubeconfig"
	// KubeletConfigPath is the path to kubelet configuration file
	KubeletConfigPath = "/etc/kubernetes/kubelet.kubeconfig"

	// DNSResourceName specifies the name for the DNS resources
	DNSResourceName = "kube-dns"

	// CoreDNSConfigMapName is the location of the user supplied configmap for CoreDNS configuration
	CoreDNSConfigMapName = "coredns"

	// ExitCodeUnknown is equivalent to EX_SOFTWARE as defined by sysexits(3)
	ExitCodeUnknown = 70

	// SharedReadMask is a file mask with read access for everyone
	SharedReadMask = 0644

	// SharedReadWriteMask is a mask for a shared file with read/write access for everyone
	SharedReadWriteMask = 0666

	// SharedDirMask is a mask for shared directories
	SharedDirMask = 0755

	// SystemdUnitPath specifies the path for user systemd units
	SystemdUnitPath = "/etc/systemd/system"

	// CloudProviderAWS defines the name of the AWS cloud provider used to
	// setup AWS integration in kubernetes
	CloudProviderAWS = "aws"
	// CloudProviderGCE is the Google Compute Engine cloud provider ID
	CloudProviderGCE = "gce"

	// OverlayInterfaceName is the name of the linux network interface connected to the overlay network
	OverlayInterfaceName = "docker0"

	// APIServerDNSName is the domain name of the current leader server.
	APIServerDNSName = "leader.telekube.local"
	// APIServerDNSNameGravity is the domain name of the current leader server.
	APIServerDNSNameGravity = "leader.gravity.local"
	// RegistryDNSName is the domain name of the cluster local registry.
	RegistryDNSName = "registry.local"

	// CloudConfigFile specifies the file path for cloud-config for the kubernetes cloud controller
	CloudConfigFile = "/etc/kubernetes/cloud-config.conf"

	// KubeletConfigFile specifies the file path for kubelet configuration
	KubeletConfigFile = "/etc/kubernetes/kubelet.yaml"
)

var (
	// GravityDataDir is the directory where gravity data is stored in planet
	GravityDataDir = "/var/lib/gravity"
)
