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

package monitoring

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/gravitational/planet/lib/constants"

	etcdconf "github.com/gravitational/coordinate/config"
	"github.com/gravitational/satellite/agent"
	"github.com/gravitational/satellite/agent/health"
	"github.com/gravitational/satellite/monitoring"
	"github.com/gravitational/trace"
	serf "github.com/hashicorp/serf/client"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Config represents configuration for setting up monitoring checkers.
type Config struct {
	// Role is the current agent's role
	Role agent.Role
	// AdvertiseIP is the planet agent's advertised IP address
	AdvertiseIP string
	// KubeAddr is the address of the kubernetes API server
	KubeAddr string
	// ClusterDNS is the IP of the kubernetes DNS service
	ClusterDNS string
	// UpstreamNameservers lists additional upstream nameserver added to the DNS configuration
	UpstreamNameservers []string
	// LocalNameservers is a list of addresses local nameserver listens on
	LocalNameservers []string
	// DNSZones maps DNS zone to a list of nameservers
	DNSZones map[string][]string
	// RegistryAddr is the address of the private docker registry
	RegistryAddr string
	// NettestContainerImage is the name of the container image used for
	// networking test
	NettestContainerImage string
	// DisableInterPodCheck disables inter-pod communication tests
	DisableInterPodCheck bool
	// ETCDConfig defines etcd-specific configuration
	ETCDConfig etcdconf.Config
	// CloudProvider is the cloud provider backend this cluster is using
	CloudProvider string
	// NodeName is the kubernetes name of this node
	NodeName string
	// HighWatermark is the usage limit percentage of monitored directories and devicemapper
	HighWatermark uint
}

// Check validates monitoring configuration
func (c Config) Check() error {
	if c.HighWatermark > 100 {
		return trace.BadParameter("high watermark percentage should be 0-100")
	}
	return nil
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

// GetKubeClient returns a Kubernetes client that uses kubectl certificate
// for authentication
func GetKubeClient() (*kubernetes.Clientset, error) {
	return getKubeClientFromPath(constants.KubeletConfigPath)
}

// GetPrivilegedKubeClient returns a Kubernetes client that uses scheduler
// certificate for authentication
func GetPrivilegedKubeClient() (*kubernetes.Clientset, error) {
	return getKubeClientFromPath(constants.SchedulerConfigPath)
}

// getKubeClientFromPath returns a Kubernetes client using the given kubeconfig file
func getKubeClientFromPath(kubeconfigPath string) (*kubernetes.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return client, nil
}

// AddCheckers adds checkers to the agent.
func AddCheckers(node agent.Agent, config *Config) (err error) {
	etcdConfig := &monitoring.ETCDConfig{
		Endpoints: config.ETCDConfig.Endpoints,
		CAFile:    config.ETCDConfig.CAFile,
		CertFile:  config.ETCDConfig.CertFile,
		KeyFile:   config.ETCDConfig.KeyFile,
	}

	switch config.Role {
	case agent.RoleMaster:
		err = addToMaster(node, config, etcdConfig)
	case agent.RoleNode:
		err = addToNode(node, config, etcdConfig)
	}
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
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

	client, err := GetKubeClient()
	if err != nil {
		return trace.Wrap(err)
	}

	// Kubelet certificate does not have permissions to query ComponentStatuses
	apiServerClient, err := getKubeClientFromPath(constants.KubectlConfigPath)
	if err != nil {
		return trace.Wrap(err)
	}

	kubeConfig := monitoring.KubeConfig{Client: client}
	node.AddChecker(monitoring.KubeAPIServerHealth(monitoring.KubeConfig{Client: apiServerClient}))
	node.AddChecker(monitoring.DockerHealth("/var/run/docker.sock"))
	node.AddChecker(dockerRegistryHealth(config.RegistryAddr, localTransport))
	node.AddChecker(etcdChecker)
	node.AddChecker(monitoring.SystemdHealth())
	node.AddChecker(monitoring.NewIPForwardChecker())
	node.AddChecker(monitoring.NewBridgeNetfilterChecker())
	node.AddChecker(monitoring.NewMayDetachMountsChecker())
	node.AddChecker(monitoring.NewInotifyChecker())
	node.AddChecker(monitoring.NewNodeStatusChecker(kubeConfig, config.NodeName))
	if !config.DisableInterPodCheck {
		node.AddChecker(monitoring.InterPodCommunication(kubeConfig, config.NettestContainerImage))
	}
	node.AddChecker(NewVersionCollector())
	if len(config.LocalNameservers) > 0 {
		node.AddChecker(monitoring.NewDNSChecker([]string{
			"leader.telekube.local.",
		}, config.LocalNameservers...))
	}
	node.AddChecker(monitoring.NewStorageChecker(monitoring.StorageConfig{
		Path:          constants.GravityDataDir,
		HighWatermark: config.HighWatermark,
	}))
	// the following checker will be no-op if docker driver is not devicemapper
	node.AddChecker(monitoring.NewDockerDevicemapperChecker(
		monitoring.DockerDevicemapperConfig{
			HighWatermark: config.HighWatermark,
		}))

	pingChecker, err := monitoring.NewPingChecker(
		monitoring.PingCheckerConfig{
			SerfRPCAddr:    node.GetConfig().SerfConfig.Addr,
			SerfMemberName: node.GetConfig().Name,
		})
	if err != nil {
		return trace.Wrap(err)
	}
	node.AddChecker(pingChecker)

	serfClient, err := agent.NewSerfClient(serf.Config{
		Addr: node.GetConfig().SerfConfig.Addr,
	})
	if err != nil {
		return trace.Wrap(err)
	}

	serfMember, err := serfClient.FindMember(node.GetConfig().Name)
	if err != nil {
		return trace.Wrap(err)
	}

	timeDriftChecker, err := monitoring.NewTimeDriftChecker(
		monitoring.TimeDriftCheckerConfig{
			CAFile:     node.GetConfig().CAFile,
			CertFile:   node.GetConfig().CertFile,
			KeyFile:    node.GetConfig().KeyFile,
			SerfClient: serfClient,
			SerfMember: serfMember,
		})
	if err != nil {
		return trace.Wrap(err)
	}
	node.AddChecker(timeDriftChecker)

	// Add checkers specific to cloud provider backend
	switch strings.ToLower(config.CloudProvider) {
	case constants.CloudProviderAWS:
		node.AddChecker(monitoring.NewAWSHasProfileChecker())
	}

	nethealthChecker, err := monitoring.NewNethealthChecker(
		monitoring.NethealthConfig{
			NodeName:   config.NodeName,
			KubeConfig: &kubeConfig,
		},
	)
	node.AddChecker(nethealthChecker)

	return nil
}

func addToNode(node agent.Agent, config *Config, etcdConfig *monitoring.ETCDConfig) error {
	etcdChecker, err := monitoring.EtcdHealth(etcdConfig)
	if err != nil {
		return trace.Wrap(err)
	}

	// Create a different client for nodes to be able to query node information
	nodeClient, err := getKubeClientFromPath(constants.KubeletConfigPath)
	if err != nil {
		return trace.Wrap(err)
	}

	nodeConfig := monitoring.KubeConfig{Client: nodeClient}
	node.AddChecker(monitoring.KubeletHealth("http://127.0.0.1:10248"))
	node.AddChecker(monitoring.DockerHealth("/var/run/docker.sock"))
	node.AddChecker(etcdChecker)
	node.AddChecker(monitoring.SystemdHealth())
	node.AddChecker(NewVersionCollector())
	node.AddChecker(monitoring.NewIPForwardChecker())
	node.AddChecker(monitoring.NewBridgeNetfilterChecker())
	node.AddChecker(monitoring.NewMayDetachMountsChecker())
	node.AddChecker(monitoring.NewInotifyChecker())
	if len(config.LocalNameservers) > 0 {
		node.AddChecker(monitoring.NewDNSChecker([]string{
			"leader.telekube.local.",
		}, config.LocalNameservers...))
	}
	node.AddChecker(monitoring.NewNodeStatusChecker(nodeConfig, config.NodeName))
	node.AddChecker(monitoring.NewStorageChecker(monitoring.StorageConfig{
		Path:          constants.GravityDataDir,
		HighWatermark: config.HighWatermark,
	}))
	// the following checker will be no-op if docker driver is not devicemapper
	node.AddChecker(monitoring.NewDockerDevicemapperChecker(
		monitoring.DockerDevicemapperConfig{
			HighWatermark: config.HighWatermark,
		}))

	// Add checkers specific to cloud provider backend
	switch strings.ToLower(config.CloudProvider) {
	case constants.CloudProviderAWS:
		node.AddChecker(monitoring.NewAWSHasProfileChecker())
	}

	nethealthChecker, err := monitoring.NewNethealthChecker(
		monitoring.NethealthConfig{
			NodeName:   config.NodeName,
			KubeConfig: &nodeConfig,
		},
	)
	node.AddChecker(nethealthChecker)

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
