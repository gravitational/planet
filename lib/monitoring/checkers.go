package monitoring

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/gravitational/planet/lib/constants"

	etcdconf "github.com/gravitational/coordinate/config"
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
	// UpstreamNameservers lists additional upstream nameserver added to the DNS configuration
	UpstreamNameservers []string
	// RegistryAddr is the address of the private docker registry
	RegistryAddr string
	// NettestContainerImage is the name of the container image used for
	// networking test
	NettestContainerImage string
	// DisableInterPodCheck disables inter-pod communication tests
	DisableInterPodCheck bool
	// ETCDConfig defines etcd-specific configuration
	ETCDConfig etcdconf.Config
}

// LocalTransport returns http transport that is set up with local certificate authority
// and client certificates
func (c *Config) LocalTransport() (*http.Transport, error) {
	cert, err := tls.LoadX509KeyPair(c.ETCDConfig.CertFile, c.ETCDConfig.KeyFile)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	roots, err := newCertPool([]string{c.ETCDConfig.CAFile})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &http.Transport{
		TLSClientConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS10,
			RootCAs:      roots,
		}}, nil
}

// AddCheckers adds checkers to the agent.
func AddCheckers(node agent.Agent, config *Config) {
	etcdConfig := &monitoring.ETCDConfig{
		Endpoints: config.ETCDConfig.Endpoints,
		CAFile:    config.ETCDConfig.CAFile,
		CertFile:  config.ETCDConfig.CertFile,
		KeyFile:   config.ETCDConfig.KeyFile,
	}
	switch config.Role {
	case agent.RoleMaster:
		addToMaster(node, config, etcdConfig)
	case agent.RoleNode:
		addToNode(node, config, etcdConfig)
	}
}

func addToMaster(node agent.Agent, config *Config, etcdConfig *monitoring.ETCDConfig) error {
	localTransport, err := config.LocalTransport()
	if err != nil {
		return trace.Wrap(err)
	}
	etcdChecker, err := monitoring.EtcdHealth(etcdConfig)
	if err != nil {
		return trace.Wrap(err)
	}
	node.AddChecker(monitoring.KubeAPIServerHealth(config.KubeAddr, constants.SchedulerConfigPath))
	node.AddChecker(monitoring.DockerHealth("/var/run/docker.sock"))
	node.AddChecker(dockerRegistryHealth(config.RegistryAddr, localTransport))
	node.AddChecker(etcdChecker)
	node.AddChecker(monitoring.SystemdHealth())
	node.AddChecker(monitoring.NewIPForwardChecker())
	node.AddChecker(monitoring.NewBrNetfilterChecker())
	if !config.DisableInterPodCheck {
		node.AddChecker(monitoring.InterPodCommunication(config.KubeAddr, config.NettestContainerImage))
	}
	node.AddChecker(NewVersionCollector())
	return nil
}

func addToNode(node agent.Agent, config *Config, etcdConfig *monitoring.ETCDConfig) error {
	etcdChecker, err := monitoring.EtcdHealth(etcdConfig)
	if err != nil {
		return trace.Wrap(err)
	}
	node.AddChecker(monitoring.KubeletHealth("http://127.0.0.1:10248"))
	node.AddChecker(monitoring.DockerHealth("/var/run/docker.sock"))
	node.AddChecker(etcdChecker)
	node.AddChecker(monitoring.SystemdHealth())
	node.AddChecker(NewVersionCollector())
	node.AddChecker(monitoring.NewIPForwardChecker())
	return nil
}

func dockerRegistryHealth(addr string, transport *http.Transport) health.Checker {
	return monitoring.NewHTTPHealthzCheckerWithTransport("docker-registry", fmt.Sprintf("%v/v2/", addr), transport, noopResponseChecker)
}

func noopResponseChecker(response io.Reader) error {
	return nil
}

// newCertPool creates x509 certPool with provided CA files.
func newCertPool(CAFiles []string) (*x509.CertPool, error) {
	certPool := x509.NewCertPool()

	for _, CAFile := range CAFiles {
		pemByte, err := ioutil.ReadFile(CAFile)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		for {
			var block *pem.Block
			block, pemByte = pem.Decode(pemByte)
			if block == nil {
				break
			}
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			certPool.AddCert(cert)
		}
	}

	return certPool, nil
}
