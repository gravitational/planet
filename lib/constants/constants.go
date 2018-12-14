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

	// ExitCodeUnknown is equivalent to EX_SOFTWARE as defined by sysexits(3)
	ExitCodeUnknown = 70

	// SharedReadMask is a file mask with read access for everyone
	SharedReadMask = 0644

	// SharedDirMask is a mask for shared directories
	SharedDirMask = 0755

	// SystemdUnitPath specifies the path for user systemd units
	SystemdUnitPath = "/etc/systemd/system"
)
