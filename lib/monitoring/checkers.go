package monitoring

import (
	"fmt"
	"io"

	"github.com/gravitational/planet/lib/etcd"
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
	// RegistryAddr is the address of the private docker registry
	RegistryAddr string
	// NettestContainerImage is the name of the container image used for
	// networking test
	NettestContainerImage string
	// Etcd defines etcd-specific configuration
	EtcdConfig etcd.Config
}

// AddCheckers adds checkers to the agent.
func AddCheckers(node agent.Agent, config *Config) {
	etcdConfig := &monitoring.EtcdConfig{
		Endpoints: config.EtcdConfig.Endpoints,
		CAFile:    config.EtcdConfig.CAFile,
		CertFile:  config.EtcdConfig.CertFile,
		KeyFile:   config.EtcdConfig.KeyFile,
	}
	switch config.Role {
	case agent.RoleMaster:
		addToMaster(node, config, etcdConfig)
	case agent.RoleNode:
		addToNode(node, config, etcdConfig)
	}
}

func addToMaster(node agent.Agent, config *Config, etcdConfig *monitoring.EtcdConfig) error {
	etcdChecker, err := monitoring.EtcdHealth(etcdConfig)
	if err != nil {
		return trace.Wrap(err)
	}
	node.AddChecker(monitoring.KubeApiServerHealth(config.KubeAddr))
	// See: https://github.com/kubernetes/kubernetes/issues/17737
	// node.AddChecker(monitoring.ComponentStatusHealth(config.KubeAddr))
	node.AddChecker(monitoring.DockerHealth("/var/run/docker.sock"))
	node.AddChecker(dockerRegistryHealth(config.RegistryAddr))
	node.AddChecker(etcdChecker)
	node.AddChecker(monitoring.SystemdHealth())
	node.AddChecker(monitoring.IntraPodCommunication(config.KubeAddr, config.NettestContainerImage))
	return nil
}

func addToNode(node agent.Agent, config *Config, etcdConfig *monitoring.EtcdConfig) error {
	etcdChecker, err := monitoring.EtcdHealth(etcdConfig)
	if err != nil {
		return trace.Wrap(err)
	}
	node.AddChecker(monitoring.KubeletHealth("http://127.0.0.1:10248"))
	node.AddChecker(monitoring.DockerHealth("/var/run/docker.sock"))
	node.AddChecker(etcdChecker)
	node.AddChecker(monitoring.SystemdHealth())
	return nil
}

func dockerRegistryHealth(addr string) health.Checker {
	return monitoring.NewHTTPHealthzChecker("docker-registry", fmt.Sprintf("%v/v2/", addr), noopResponseChecker)
}

func noopResponseChecker(response io.Reader) error {
	return nil
}
