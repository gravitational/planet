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

import (
	"time"

	"github.com/gravitational/satellite/monitoring"
)

const (
	// KubectlConfigPath is the path to kubectl configuration file
	KubectlConfigPath = "/etc/kubernetes/kubectl.kubeconfig"
	// KubectlHostConfigPath is the path to configuration file that kubectl
	// uses when invoked from host
	KubectlHostConfigPath = "/etc/kubernetes/kubectl-host.kubeconfig"
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

	// GroupReadWriteMask is a file mask for owder/group read/write
	GroupReadWriteMask = 0660

	// OwnerReadMask is a file mask for owner read-only
	OwnerReadMask = 0400

	// DeviceReadWritePerms specifies the read/write permissions for a device
	DeviceReadWritePerms = "rwm"

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
	// APIServerPort is the default secure port for the api server
	APIServerPort = "6443"
	// RegistryDNSName is the domain name of the cluster local registry.
	RegistryDNSName = "registry.local"

	// CloudConfigFile specifies the file path for cloud-config for the kubernetes cloud controller
	CloudConfigFile = "/etc/kubernetes/cloud-config.conf"

	// KubeletConfigFile specifies the file path for kubelet configuration
	KubeletConfigFile = "/etc/kubernetes/kubelet.yaml"

	// HTTPTimeout specifies the default HTTP timeout for checks
	HTTPTimeout = 10 * time.Second

	// ProxyEnvironmentFile is an environment file with outbound proxy options
	// Note: these settings are separate from container-environment because not all processes should load the proxy
	// settings
	ProxyEnvironmentFile = "/etc/proxy-environment"

	// ContainerRuntimeProcessLabel specifies the SELinux label for the planet process
	ContainerRuntimeProcessLabel = "system_u:system_r:gravity_container_runtime_t:s0"

	// ContainerInitProcessLabel specifies the SELinux label for the init process
	ContainerInitProcessLabel = "system_u:system_r:gravity_container_init_t:s0"

	// EncryptionProviderConfig specifies the path to the encryption provider configuration.
	EncryptionProviderConfig = "/etc/kubernetes/encryption-configuration.yaml"

	// CredentialProviderConfig specifies the path to the credential provider configuration.
	CredentialProviderConfig = "/etc/kubernetes/credential-provider-configuration.yaml"

	// CredentialProviderBinDir specifies the path to the directory with credential provider plugin binaries.
	CredentialProviderBinDir = "/opt/ecr-credential-provider/bin/"
)

var (
	// GravityDataDir is the directory where gravity data is stored in planet
	GravityDataDir = "/var/lib/gravity"

	// MinKernelVersion is the minimum supported Linux kernel version -- 3.10.0-1127.
	//
	// Some clusters running on an older CentOS/RHEL kernel have experienced
	// instability and memory allocation failures.
	// See https://github.com/gravitational/gravity/issues/1818 for more information.
	MinKernelVersion = monitoring.KernelVersion{
		Release: 3,
		Major:   10,
		Minor:   0,
		Patch:   1127,
	}
)
