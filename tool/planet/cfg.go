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

package main

import (
	"bytes"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/gravitational/planet/lib/box"
	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/user"
	"github.com/gravitational/trace"

	kv "github.com/gravitational/configure"
	"github.com/gravitational/configure/cstrings"
)

// Config describes the configuration for the container start operation
type Config struct {
	// Roles specifies the list of roles this node is attached
	Roles list
	// Rootfs is the path to container's rootfs directory
	Rootfs string
	// SocketPath is the path to the container's communication socket
	SocketPath string
	// PublicIP is the public IP address of this node
	PublicIP string
	// MasterIP is the IP addess of the leader
	MasterIP string
	// CloudProvider specifies the name of the cloud provider. Optional
	CloudProvider string
	// ClusterID is the unique cluster name
	ClusterID string
	// GCENodeTags specify tags to set in the cloud configuration on GCE.
	// Kubernetes will use the tags to match instances when creating LoadBalancers on GCE.
	// By default, a cluster name is used a node tag.
	// GCE imposes restrictions on tag values and cluster names are not always conforming.
	// GCENodeTags can specify alternative node tag for LoadBalancer matching.
	GCENodeTags string
	// Env specifies the container's additional environment
	Env box.EnvVars
	// ProxyEnv specifies the containers proxy related environment variables
	ProxyEnv box.EnvVars
	// Mounts specifies the list of additional mounts
	Mounts box.Mounts
	// Devices is the list of devices to create inside container
	Devices box.Devices
	// Files are files to be shared inside the container
	Files []box.File
	// IgnoreChecks disables kernel checks during start up
	IgnoreChecks bool
	// SecretsDir specifies the location on the host with certificates.
	// This is mapped inside the container as DefaultSecretsMountDir.
	SecretsDir string
	// DockerBackend specifies the storage backend for docker
	DockerBackend string
	// DockerOptions is a list of additional docker options
	DockerOptions string
	// ServiceCIDR defines the kubernetes service subnet CIDR
	ServiceCIDR kv.CIDR
	// PodCIDR defines the kubernetes Pod subnet CIDR
	PodCIDR kv.CIDR
	// ProxyPortRange specifies the range of host ports (beginPort-endPort, single port or beginPort+offset, inclusive)
	// that may be consumed in order to proxy service traffic.
	// If (unspecified, 0, or 0-0) then ports will be randomly chosen.
	ProxyPortRange string
	// ServiceNodePortRange defines the range of ports to reserve for services with NodePort visibility.
	// Inclusive at both ends of the range.
	ServiceNodePortRange string
	// FeatureGates defines the set of key=value pairs that describe feature gates for alpha/experimental features.
	FeatureGates string
	// VxlanPort is the overlay network port
	VxlanPort int
	// InitialCluster is the initial cluster configuration for etcd
	InitialCluster kv.KeyVal
	// EtcdProxy configures the value of ETCD_PROXY environment variable
	// inside the container
	// See https://coreos.com/etcd/docs/latest/v2/configuration.html for details
	EtcdProxy string
	// EtcdMemberName configures the value of ETCD_MEMBER_NAME environment variable
	// inside the container
	EtcdMemberName string
	// EtcdInitialCluster configures the value of ETCD_INITIAL_CLUSTER environment variable
	// inside the container
	EtcdInitialCluster string
	// EtcdGatewayList is a list of etcd endpoints that the etcd gateway can use to reach the cluster
	EtcdGatewayList string
	// EtcdInitialClusterState configures the value of ETCD_INITIAL_CLUSTER_STATE environment variable
	// inside the container
	EtcdInitialClusterState string
	// EtcdOptions specifies additional command line options to etcd daemon
	EtcdOptions string
	// ElectionEnabled specifies if this planet node participates in leader election
	ElectionEnabled bool
	// NodeName overrides the name of the node for kubernetes
	NodeName string
	// Hostname specifies the new hostname inside the container
	Hostname string
	// KubeletOptions defines additional kubelet parameters
	KubeletOptions string
	// APIServerOptions defines additional parameters for API server
	APIServerOptions string
	// ServiceUser defines the user context for container's service user
	ServiceUser serviceUser
	// DNS is the local DNS configuration
	DNS DNS
	// DisableFlannel tells planet to disable the embedded flannel plugin
	DisableFlannel bool
	// KubeletConfig specifies the configuration for kubelet as JSON-encoded payload
	KubeletConfig string
	// CloudConfig specifies the cloud configuration as JSON-encoded payload
	CloudConfig string
	// AllowPrivileged controls whether privileged containers are allowed.
	AllowPrivileged bool
}

// DNS describes DNS server configuration
type DNS struct {
	// Hosts is a host->ip mapping
	Hosts box.DNSOverrides
	// Zones is a zone->nameserver mapping
	Zones box.DNSOverrides
	// ListenAddrs specifies the IP addresses for CoreDNS to listen on
	ListenAddrs []string
	// Port specifies the DNS port
	Port int
}

func (cfg *Config) checkAndSetDefaults() (err error) {
	cfg.ServiceUser.User, err = user.LookupID(cfg.ServiceUser.UID)
	if err != nil {
		return trace.Wrap(err)
	}
	if cfg.VxlanPort <= 0 {
		cfg.VxlanPort = DefaultVxlanPort
	}
	return nil
}

type serviceUser struct {
	*user.User
	UID string
}

func (cfg *Config) KubeDNSResolverIP() string {
	return cfg.ServiceCIDR.RelativeIP(3).String()
}

// APIServerIP returns the IP of the "kubernetes" service which is the first IP
// of the configured service subnet
func (cfg *Config) APIServerIP() net.IP {
	return cfg.ServiceCIDR.FirstIP()
}

// HostStateDir returns the gravity state directory on host.
func (cfg *Config) HostStateDir() string {
	// Host's state directory can be customized but it's always mounted
	// as /var/lib/gravity inside planet container so to find the state
	// directory on host, find the source for /var/lib/gravity.
	for _, mount := range cfg.Mounts {
		if mount.Dst == constants.GravityDataDir {
			return mount.Src
		}
	}
	// Should not reach this b/c /var/lib/gravity is always mounted,
	// but fallback to default just in case.
	return constants.GravityDataDir
}

func (cfg *Config) hasRole(r string) bool {
	for _, rs := range cfg.Roles {
		if rs == r {
			return true
		}
	}
	return false
}

func (cfg *Config) inRootfs(paths ...string) string {
	return filepath.Join(append([]string{cfg.Rootfs}, paths...)...)
}

type list []string

// IsCumulative determines if this flag can be specified multiple times.
// Implements kingpin.repeatableFlag
func (l *list) IsCumulative() bool {
	return true
}

// Set sets the value for this flag from command line
func (l *list) Set(val string) error {
	for _, r := range cstrings.SplitComma(val) {
		*l = append(*l, r)
	}
	return nil
}

// String returns a textual representation of the flag
func (l *list) String() string {
	return fmt.Sprintf("%v", []string(*l))
}

// hostPort is a command line flag that understands input
// as a host:port pair.
type hostPort struct {
	host string
	port int64
}

func (r *hostPort) Set(input string) error {
	var err error
	var port string

	r.host, port, err = net.SplitHostPort(input)
	if err != nil {
		return err
	}

	r.port, err = strconv.ParseInt(port, 0, 0)
	return err
}

func (r hostPort) String() string {
	return net.JoinHostPort(r.host, fmt.Sprintf("%v", r.port))
}

// toKeyValueList combines key/value pairs from kv into a comma-separated list.
func toKeyValueList(kv kv.KeyVal) string {
	var result []string
	for key, value := range kv {
		result = append(result, fmt.Sprintf("%v:%v", key, value))
	}
	return strings.Join(result, ",")
}

// boolFlag defines a boolean command line flag.
// The behavioral difference to the kingpin's built-in Bool() modifier
// is that it supports the long form:
// 	--flag=true|false
// as opposed to built-in's only short form:
//	--flag	(true, if specified, false - otherwise)
// The long form is required when populating the flag from the environment.
type boolFlag bool

func (r *boolFlag) Set(input string) error {
	if input == "" {
		input = "true"
	}
	value, err := strconv.ParseBool(input)
	*r = boolFlag(value)
	return err
}

func (r boolFlag) String() string {
	return strconv.FormatBool(bool(r))
}

// newKubectlConfig returns a kubectl config for the specified kubernetes API server IP
func newKubectlConfig(ip net.IP, stateDir string) ([]byte, error) {
	var b bytes.Buffer
	err := kubectlConfig.Execute(&b, map[string]string{
		"ip":       ip.String(),
		"stateDir": stateDir,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return b.Bytes(), nil
}

// kubectlConfig is a template of a configuration file for kubectl
var kubectlConfig = template.Must(template.New("kubeConfig").Parse(`apiVersion: v1
kind: Config
current-context: default
clusters:
- name: default
  cluster:
    certificate-authority: {{.stateDir}}/secrets/root.cert
    server: https://{{.ip}}
users:
- name: default
  user:
    client-certificate: {{.stateDir}}/secrets/kubectl.cert
    client-key: {{.stateDir}}/secrets/kubectl.key
contexts:
- name: default
  context:
    cluster: default
    user: default
    namespace: default`))
