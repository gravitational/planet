package monitoring

import (
	"io"

	"github.com/gravitational/satellite/agent"
	"github.com/gravitational/satellite/agent/health"
	"github.com/gravitational/satellite/monitoring"
	"github.com/gravitational/trace"
)

// Config represents configuration for setting up monitoring checkers.
type Config struct {
	// Role is the current agent's role
	Role agent.Role
	// KubeAddr is the address of the kubernetes API server
	KubeAddr string
	// ClusterDNS is the IP of the kubernetes DNS service
	ClusterDNS string
	// NettestContainerImage is the name of the container image used for
	// networking test
	NettestContainerImage string
	// Etcd defines etcd-specific configuration
	Etcd EtcdConfig
}

// EtcdConfig defines etcd-specific configuration
type EtcdConfig struct {
	// TLSConfig defines configuration for securing etcd communication
	TLSConfig *monitoring.TLSConfig
}

// AddCheckers adds checkers to the agent.
func AddCheckers(node agent.Agent, config *Config) {
	switch config.Role {
	case agent.RoleMaster:
		addToMaster(node, config)
	case agent.RoleNode:
		addToNode(node, config)
	}
}

func addToMaster(node agent.Agent, config *Config) error {
	etcdChecker, err := monitoring.EtcdHealth("https://127.0.0.1:2379", config.Etcd.TLSConfig)
	if err != nil {
		return trace.Wrap(err)
	}
	node.AddChecker(monitoring.KubeApiServerHealth(config.KubeAddr))
	// See: https://github.com/kubernetes/kubernetes/issues/17737
	// node.AddChecker(monitoring.ComponentStatusHealth(config.KubeAddr))
	node.AddChecker(monitoring.DockerHealth("/var/run/docker.sock"))
	node.AddChecker(dockerRegistryHealth())
	node.AddChecker(etcdChecker)
	node.AddChecker(monitoring.SystemdHealth())
	node.AddChecker(monitoring.IntraPodCommunication(config.KubeAddr, config.NettestContainerImage))
	return nil
}

func addToNode(node agent.Agent, config *Config) error {
	etcdChecker, err := monitoring.EtcdHealth("https://127.0.0.1:2379", config.Etcd.TLSConfig)
	if err != nil {
		return trace.Wrap(err)
	}
	node.AddChecker(monitoring.KubeletHealth("http://127.0.0.1:10248"))
	node.AddChecker(monitoring.DockerHealth("/var/run/docker.sock"))
	node.AddChecker(etcdChecker)
	node.AddChecker(monitoring.SystemdHealth())
	return nil
}

func dockerRegistryHealth() health.Checker {
	return monitoring.NewHTTPHealthzChecker("docker-registry", "http://127.0.0.1:5000/v2/", noopResponseChecker)
}

func noopResponseChecker(response io.Reader) error {
	return nil
}
