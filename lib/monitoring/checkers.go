package monitoring

import (
	"io"

	"github.com/gravitational/satellite/agent"
	"github.com/gravitational/satellite/agent/health"
	"github.com/gravitational/satellite/monitoring"
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

func addToMaster(node agent.Agent, config *Config) {
	node.AddChecker(monitoring.KubeApiServerHealth(config.KubeAddr))
	node.AddChecker(monitoring.ComponentStatusHealth(config.KubeAddr))
	node.AddChecker(monitoring.DockerHealth("unix://var/run/docker.sock"))
	node.AddChecker(dockerRegistryHealth())
	node.AddChecker(monitoring.EtcdHealth("http://127.0.0.1:2379"))
	node.AddChecker(monitoring.SystemdHealth())
	node.AddChecker(monitoring.IntraPodCommunication(config.KubeAddr, config.NettestContainerImage))
}

func addToNode(node agent.Agent, confg *Config) {
	node.AddChecker(monitoring.KubeletHealth("http://127.0.0.1:10248"))
	node.AddChecker(monitoring.DockerHealth("unix://var/run/docker.sock"))
	node.AddChecker(monitoring.EtcdHealth("http://127.0.0.1:2379"))
	node.AddChecker(monitoring.SystemdHealth())
}

func dockerRegistryHealth() health.Checker {
	return monitoring.NewHTTPHealthzChecker("docker-registry", "http://127.0.0.1:5000/v2/", noopResponseChecker)
}

func noopResponseChecker(response io.Reader) error {
	return nil
}
