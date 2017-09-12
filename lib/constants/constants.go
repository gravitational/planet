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
)
