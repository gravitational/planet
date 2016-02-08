package monitoring

import (
	"github.com/gravitational/planet/lib/agent"
	"github.com/gravitational/planet/lib/agent/health"
)

// Config represents configuration for setting up monitoring checkers.
type Config struct {
	// Role is the current agent's role
	Role agent.Role
	// KubeAddr is the address of the kubernetes API server
	KubeAddr string
	// ClusterDNS is the IP of the kubernetes DNS service
	ClusterDNS string
	// RegistryAddr is the address of private docker registry
	RegistryAddr string
}

// AddCheckers adds checkers to the agent.
func AddCheckers(node agent.Agent, conf *Config) {
	switch conf.Role {
	case agent.RoleMaster:
		addToMaster(node, conf)
	case agent.RoleNode:
		addToNode(node, conf)
	}
}

func addToMaster(node agent.Agent, conf *Config) {
	node.AddChecker(kubeApiServerHealth())
	node.AddChecker(componentStatusHealth(conf.KubeAddr))
	node.AddChecker(dockerHealth())
	node.AddChecker(dockerRegistryHealth())
	node.AddChecker(etcdHealth())
	node.AddChecker(systemdHealth())
	node.AddChecker(intraPodCommunication(conf.KubeAddr, conf.RegistryAddr))
}

func addToNode(node agent.Agent, conf *Config) {
	node.AddChecker(kubeletHealth())
	node.AddChecker(dockerHealth())
	node.AddChecker(etcdHealth())
	node.AddChecker(systemdHealth())
}

func kubeApiServerHealth() health.Checker {
	return newChecker(newHTTPHealthzChecker("http://127.0.0.1:8080/healthz", kubeHealthz), "kube-apiserver")
}

func kubeletHealth() health.Checker {
	return newChecker(newHTTPHealthzChecker("http://127.0.0.1:10248/healthz", kubeHealthz), "kubelet")
}

func componentStatusHealth(kubeAddr string) health.Checker {
	return newChecker(&componentStatusChecker{hostPort: kubeAddr}, "componentstatuses")
}

func etcdHealth() health.Checker {
	return newChecker(newHTTPHealthzChecker("http://127.0.0.1:2379/health", etcdChecker), "etcd-healthz")
}

func dockerHealth() health.Checker {
	return newChecker(newUnixSocketHealthzChecker("http://docker/version", "/var/run/docker.sock",
		dockerChecker), "docker")
}

func dockerRegistryHealth() health.Checker {
	return newChecker(newHTTPHealthzChecker("http://127.0.0.1:5000/v2/", dockerChecker), "docker-registry")
}

func systemdHealth() health.Checker {
	return newChecker(systemdChecker{}, "systemd")
}

func intraPodCommunication(kubeAddr, registryAddr string) health.Checker {
	return newChecker(newIntraPodChecker(kubeAddr, registryAddr), "networking")
}
