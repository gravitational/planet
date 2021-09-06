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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/gravitational/planet/lib/box"
	"github.com/gravitational/planet/lib/check"
	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/defaults"
	"github.com/gravitational/planet/lib/user"
	"github.com/gravitational/planet/lib/utils"

	"github.com/ghodss/yaml"
	"github.com/gravitational/trace"
	"github.com/imdario/mergo"
	log "github.com/sirupsen/logrus"
	kubeletconfig "k8s.io/kubelet/config/v1beta1"
)

// runtimeContext defines the context of a running planet container
type runtimeContext struct {
	// process is the main planet process
	process *box.Box
	// listener is the udev device listener
	listener io.Closer
}

// Close closes the container process and stops the udev listener
func (r *runtimeContext) Close() error {
	r.listener.Close()
	return r.process.Close()
}

func startAndWait(config *Config) error {
	if err := config.checkAndSetDefaults(); err != nil {
		return trace.Wrap(err)
	}

	ctx, err := start(config)
	if err != nil {
		return trace.Wrap(err)
	}
	defer ctx.Close()

	// wait for the process to finish.
	status, err := ctx.process.Wait()
	if err != nil {
		return trace.Wrap(err)
	}
	log.WithField("status", status).Info("box.Wait() finished")
	return nil
}

func start(config *Config) (*runtimeContext, error) {
	log.Infof("Starting with config: %#v.", config)

	if !config.SELinux && !isRoot() {
		return nil, trace.CompareFailed("must be run as root")
	}

	var err error

	// see if the kernel version is supported:
	v, err := check.KernelVersion()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	log.Infof("Kernel: %v.", v)
	if v < MinKernelVersion {
		err := trace.Errorf(
			"current minimum supported kernel version is %0.2f. Upgrade kernel before moving on.", MinKernelVersion/100.0)
		if !config.IgnoreChecks {
			return nil, trace.Wrap(err)
		}
		log.WithError(err).Warn("Ignore kernel supported version check.")
	}

	// check & mount cgroups:
	if err = box.MountCgroups("/"); err != nil {
		return nil, trace.Wrap(err)
	}

	if config.DockerBackend == "" {
		// check supported storage back-ends for docker
		config.DockerBackend, err = pickDockerStorageBackend()
		if err != nil {
			return nil, trace.Wrap(err)
		}
	}

	// add service user/group to container
	err = addUserToContainer(config.Rootfs, config.ServiceUser)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	err = addGroupToContainer(config.Rootfs, config.ServiceUser)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// validate the mounts:
	if err = checkRequiredMounts(config); err != nil {
		return nil, trace.Wrap(err)
	}
	// make sure the role is set
	if !config.hasRole(RoleMaster) && !config.hasRole(RoleNode) {
		return nil, trace.Errorf("--role parameter must be set")
	}

	config.Env = append(config.Env,
		box.EnvPair{Name: EnvUpgradeFrom, Val: config.UpgradeFrom},
		box.EnvPair{Name: EnvMasterIP, Val: config.MasterIP},
		box.EnvPair{Name: EnvCloudProvider, Val: config.CloudProvider},
		box.EnvPair{Name: EnvServiceSubnet, Val: config.ServiceCIDR.String()},
		box.EnvPair{Name: EnvPodSubnet, Val: config.PodCIDR.String()},
		box.EnvPair{Name: EnvPodSubnetSize, Val: strconv.Itoa(config.PodSubnetSize)},
		box.EnvPair{Name: EnvServiceNodePortRange, Val: config.ServiceNodePortRange},
		box.EnvPair{Name: EnvProxyPortRange, Val: config.ProxyPortRange},
		box.EnvPair{Name: EnvPublicIP, Val: config.PublicIP},
		box.EnvPair{Name: EnvVxlanPort, Val: strconv.Itoa(config.VxlanPort)},
		// Default agent name to the name of the etcd member
		box.EnvPair{Name: EnvAgentName, Val: config.EtcdMemberName},
		box.EnvPair{Name: EnvInitialCluster, Val: toKeyValueList(config.InitialCluster)},
		box.EnvPair{Name: EnvAPIServerName, Val: constants.APIServerDNSName},
		box.EnvPair{Name: EnvAPIServerPort, Val: constants.APIServerPort},
		box.EnvPair{Name: EnvEtcdProxy, Val: config.EtcdProxy},
		box.EnvPair{Name: EnvEtcdMemberName, Val: config.EtcdMemberName},
		box.EnvPair{Name: EnvEtcdInitialCluster, Val: config.EtcdInitialCluster},
		box.EnvPair{Name: EnvEtcdGatewayEndpoints, Val: config.EtcdGatewayList},
		box.EnvPair{Name: EnvEtcdInitialClusterState, Val: config.EtcdInitialClusterState},
		box.EnvPair{Name: EnvRole, Val: config.Roles[0]},
		box.EnvPair{Name: EnvClusterID, Val: config.ClusterID},
		box.EnvPair{Name: EnvNodeName, Val: config.NodeName},
		box.EnvPair{Name: EnvElectionEnabled, Val: strconv.FormatBool(config.ElectionEnabled)},
		box.EnvPair{Name: EnvDNSHosts, Val: config.DNS.Hosts.String()},
		box.EnvPair{Name: EnvDNSZones, Val: config.DNS.Zones.String()},
		box.EnvPair{Name: EnvPlanetAllowPrivileged, Val: strconv.FormatBool(config.AllowPrivileged)},
		box.EnvPair{Name: EnvServiceUID, Val: config.ServiceUser.UID},
		box.EnvPair{Name: EnvServiceGID, Val: config.ServiceUser.GID},
		box.EnvPair{Name: EnvHighAvailability, Val: strconv.FormatBool(config.HighAvailability)},
	)

	// Setup http_proxy / no_proxy environment configuration
	configureProxy(config)

	if err = addDockerOptions(config); err != nil {
		return nil, trace.Wrap(err)
	}
	addEtcdOptions(config)
	if err := addComponentOptions(config); err != nil {
		return nil, trace.Wrap(err)
	}
	if err = setupFlannel(config); err != nil {
		return nil, trace.Wrap(err)
	}
	if err = addCloudOptions(config); err != nil {
		return nil, trace.Wrap(err)
	}

	err = setupEtcd(config)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	upstreamNameservers, err := addResolv(config)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	// Add upstream nameservers to the cluster DNS configuration
	config.Env = append(config.Env,
		box.EnvPair{
			Name: EnvDNSUpstreamNameservers,
			Val:  strings.Join(upstreamNameservers, ","),
		})

	// Add address of local nameserver to environment so that processes
	// launched within planet such as planet-agent can locate the local
	// nameserver for testing.
	localNameservers := make([]string, 0, len(config.DNS.ListenAddrs))
	for _, address := range config.DNS.ListenAddrs {
		localNameservers = append(localNameservers, fmt.Sprintf("%v:%v",
			address, config.DNS.Port))
	}
	config.Env = append(config.Env,
		box.EnvPair{
			Name: EnvDNSLocalNameservers,
			Val:  strings.Join(localNameservers, ","),
		})

	if len(config.Taints) > 0 {
		config.Env = append(config.Env,
			box.EnvPair{
				Name: EnvPlanetTaints,
				Val:  strings.Join(config.Taints, ","),
			})
	}

	if len(config.NodeLabels) > 0 {
		config.Env = append(config.Env,
			box.EnvPair{
				Name: EnvPlanetNodeLabels,
				Val:  strings.Join(config.NodeLabels, ","),
			})
	}

	if err = setCoreDNS(config); err != nil {
		return nil, trace.Wrap(err)
	}

	if err = addKubeConfig(config); err != nil {
		return nil, trace.Wrap(err)
	}

	if !config.SELinux {
		if err = setKubeConfigOwnership(config); err != nil {
			return nil, trace.Wrap(err)
		}
	}
	mountSecrets(config)

	err = setHosts(config, generateHosts())
	if err != nil {
		return nil, trace.Wrap(err)
	}

	if config.Hostname != "" {
		// Set hostname
		config.Files = append(config.Files, box.File{
			Path:     HostnameFile,
			Contents: strings.NewReader(config.Hostname),
			Mode:     constants.SharedReadWriteMask,
		})
	}

	cfg := box.Config{
		Rootfs: config.Rootfs,
		EnvFiles: []box.EnvFile{
			{
				Path: ContainerEnvironmentFile,
				Env:  config.Env,
			},
			{
				Path: constants.ProxyEnvironmentFile,
				Env:  config.ProxyEnv,
			},
		},
		Files:        config.Files,
		Mounts:       config.Mounts,
		Devices:      config.Devices,
		DataDir:      defaults.RuncDataDir,
		InitUser:     defaults.InitUser,
		InitArgs:     defaults.InitArgs,
		InitEnv:      []string{"container=container-other", "LC_ALL=en_US.UTF-8"},
		Capabilities: allCaps,
		ProcessLabel: constants.ContainerInitProcessLabel,
		SELinux:      config.SELinux,
	}

	listener, err := newUdevListener(config.SELinux)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// start the container:
	box, err := box.Start(cfg)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	units := nodeUnits
	if config.hasRole(RoleMaster) {
		units = masterUnits
	}
	go monitorUnits(box, units...)

	return &runtimeContext{
		process:  box,
		listener: listener,
	}, nil
}

// addUserToContainer adds a record for the specified service user to the
// container's /etc/passwd
func addUserToContainer(rootfs string, u serviceUser) error {
	passwdFile, err := user.NewPasswdFromFile(filepath.Join(rootfs, UsersDatabase))
	if err != nil {
		return trace.Wrap(err)
	}
	u.Name = ServiceUser
	passwdFile.Upsert(*u.User)
	writer, err := os.OpenFile(filepath.Join(rootfs, UsersDatabase), os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return trace.Wrap(err)
	}
	defer writer.Close()
	_, err = passwdFile.WriteTo(writer)
	return trace.Wrap(err)
}

// addGroupToContainer adds a record for the specified service user to the
// container's /etc/group
func addGroupToContainer(rootfs string, u serviceUser) error {
	groupFile, err := user.NewGroupFromFile(filepath.Join(rootfs, GroupsDatabase))
	if err != nil {
		return trace.Wrap(err)
	}
	group, err := u.Group()
	if err != nil {
		return trace.Wrap(err)
	}
	group.Name = ServiceGroup
	groupFile.Upsert(*group)
	writer, err := os.OpenFile(filepath.Join(rootfs, GroupsDatabase), os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return trace.Wrap(err)
	}
	defer writer.Close()
	_, err = groupFile.WriteTo(writer)
	return trace.Wrap(err)
}

// addCloudOptions sets up cloud flags and files passed to kubernetes
// binaries, sets up container environment files
func addCloudOptions(c *Config) error {
	if c.CloudProvider == "" {
		return nil
	}

	contents, err := generateCloudConfig(c)
	if err != nil {
		return trace.Wrap(err)
	}
	c.Files = append(c.Files, box.File{
		Path:     constants.CloudConfigFile,
		Contents: strings.NewReader(contents),
	})

	c.Env.Upsert(EnvKubeCloudOptions,
		fmt.Sprintf("--cloud-provider=%v --cloud-config=%v", c.CloudProvider, constants.CloudConfigFile))

	return nil
}

// configureProxy separates http_proxy environment variables into their own config file, and appends no_proxy
// directives that may be missing in customer provided configuration
func configureProxy(c *Config) {
	// Attempt to find any current proxy settings in the variable names golang supports
	// https://github.com/golang/net/blob/c21de06aaf072cea07f3a65d6970e5c7d8b6cd6d/http/httpproxy/proxy.go#L91-L98
	proxy := []string{
		"HTTP_PROXY",
		"http_proxy",
		"HTTPS_PROXY",
		"https_proxy",
	}

	noProxy := map[string]string{
		"NO_PROXY": c.Env.Delete("NO_PROXY"),
		"no_proxy": c.Env.Delete("no_proxy"),
	}

	staticNoProxyRules := []string{"0.0.0.0/0", ".local"}

	found := false
	for k, v := range noProxy {
		if len(v) != 0 {
			// TODO(knisbet) we should see if there is a way to not NO_PROXY every IP address
			// but it's difficult because we need to know each of the nodes IP addresses, which can be added
			// after the cluster starts. Alternatively we would need to make internal connections to <ip>.ip.local and
			// have coredns convert the IP addresses for us as a DNS query.
			c.ProxyEnv.Upsert(k, strings.Join(append(staticNoProxyRules, v), ","))
			found = true
		}
	}

	// Move proxy settings from regular Environment to Proxy environment settings so not all planet processes
	// try and use the http_proxy.
	for _, k := range proxy {
		c.ProxyEnv.Upsert(k, c.Env.Delete(k))
	}

	// If we're unable to locate a NO_PROXY config, create default settings
	if !found {
		c.ProxyEnv.Upsert("NO_PROXY", strings.Join(staticNoProxyRules, ","))
	}
}

func generateCloudConfig(config *Config) (cloudConfig string, err error) {
	if config.CloudConfig != "" {
		decoded, err := base64.StdEncoding.DecodeString(config.CloudConfig)
		if err != nil {
			return "", trace.Wrap(err, "invalid cloud configuration: expected base64-encoded payload")
		}
		return string(decoded), nil
	}
	if config.ClusterID == "" {
		return "", trace.BadParameter("missing clusterID")
	}
	var buf bytes.Buffer
	switch config.CloudProvider {
	case constants.CloudProviderAWS:
		err = awsCloudConfigTemplate.Execute(&buf, config)
	case constants.CloudProviderGCE:
		err = gceCloudConfigTemplate.Execute(&buf, config)
	default:
		return "", trace.BadParameter("unsupported cloud provider %q", config.CloudProvider)
	}
	if err != nil {
		return "", trace.Wrap(err)
	}
	return buf.String(), nil
}

// pickDockerStorageBackend examines the filesystems this host supports and picks one
// suitable to be a docker storage backend, or returns an error if doesn't find a supported FS
func pickDockerStorageBackend() (dockerBackend string, err error) {
	// these backends will be tried in the order of preference:
	supportedBackends := []string{
		"overlay",
		"aufs",
	}
	for _, fs := range supportedBackends {
		ok, err := check.CheckFS(fs)
		if err != nil {
			return "", trace.Wrap(err)
		}
		// found supported FS:
		if ok {
			return fs, nil
		}
	}
	// if we get here, it means no suitable FS has been found
	err = trace.Errorf("none of the required filesystems are supported by this host: %q",
		supportedBackends)
	return "", err
}

// addDockerStorage adds a given docker storage back-end to DOCKER_OPTS environment
// variable
func addDockerOptions(config *Config) error {
	// add supported storage backend
	config.Env.Append(EnvDockerOptions,
		fmt.Sprintf("--storage-driver=%s", config.DockerBackend))

	// use cgroups native driver, because of this:
	// https://github.com/docker/docker/issues/16256
	config.Env.Append(EnvDockerOptions, "--exec-opt native.cgroupdriver=cgroupfs")
	// Add sensible size limits to logging driver
	config.Env.Append(EnvDockerOptions, "--log-opt max-size=50m")
	config.Env.Append(EnvDockerOptions, "--log-opt max-file=9")
	if config.DockerOptions != "" {
		config.Env.Append(EnvDockerOptions, config.DockerOptions)
	}
	if config.SELinux {
		config.Env.Append(EnvDockerOptions, "--selinux-enabled")
	}

	return nil
}

// addEtcdOptions sets extra etcd command line arguments in environment
func addEtcdOptions(config *Config) {
	if config.EtcdOptions != "" {
		config.Env.Append(EnvEtcdOptions, config.EtcdOptions)
	}
}

// setupEtcd runs setup tasks for etcd.
// If this is a proxy node, symlink in the etcd gateway dropin, so the etcd service runs the gateway and not etcd
// If this is a master node, and we don't detect an existing data directory, start the latest etcd, since we default
// to using the oldest etcd during an upgrade
func setupEtcd(config *Config) error {
	dropinPath := path.Join(config.Rootfs, ETCDGatewayDropinPath)

	if strings.ToLower(config.EtcdProxy) != "on" {
		err := os.Remove(dropinPath)
		if err != nil && !os.IsNotExist(err) {
			return trace.Wrap(err)
		}
		return nil
	}

	err := os.MkdirAll(path.Join(config.Rootfs, "etc/systemd/system/etcd.service.d/"), 0755)
	if err != nil {
		return trace.Wrap(err)
	}

	err = os.Symlink(
		"/lib/systemd/system/etcd-gateway.dropin",
		dropinPath,
	)
	if err != nil && !os.IsExist(err) {
		return trace.Wrap(err)
	}

	return nil
}

func addComponentOptions(config *Config) error {
	if err := addKubeletOptions(config); err != nil {
		return trace.Wrap(err)
	}
	if config.APIServerOptions != "" {
		config.Env.Append(EnvAPIServerOptions, config.APIServerOptions)
	}
	if config.ServiceNodePortRange != "" {
		config.Env.Append(EnvAPIServerOptions,
			fmt.Sprintf("--service-node-port-range=%v", config.ServiceNodePortRange))
	}
	if config.HighAvailability {
		config.Env.Append(EnvAPIServerOptions, "--endpoint-reconciler-type=lease")
	} else {
		config.Env.Append(EnvAPIServerOptions, "--endpoint-reconciler-type=master-count")
		config.Env.Append(EnvAPIServerOptions, "--apiserver-count=1")
	}
	if config.ProxyPortRange != "" {
		config.Env.Append(EnvKubeProxyOptions,
			fmt.Sprintf("--proxy-port-range=%v", config.ProxyPortRange))
	}
	if config.FeatureGates != "" {
		config.Env.Append(EnvKubeComponentFlags, fmt.Sprintf("--feature-gates=%v", config.FeatureGates))
	}
	return nil
}

// addKubeletOptions sets extra kubelet command line arguments in environment
func addKubeletOptions(config *Config) error {
	if config.KubeletOptions != "" {
		config.Env.Append(EnvKubeletOptions, config.KubeletOptions)
	}
	kubeletConfig := KubeletConfig
	if config.KubeletConfig != "" {
		decoded, err := base64.StdEncoding.DecodeString(config.KubeletConfig)
		if err != nil {
			return trace.Wrap(err, "invalid kubelet configuration: expected base64-encoded payload")
		}
		configBytes, err := utils.ToJSON([]byte(decoded))
		if err != nil {
			return trace.Wrap(err, "invalid kubelet configuration: expected either JSON or YAML")
		}
		var externalConfig kubeletconfig.KubeletConfiguration
		if err := json.Unmarshal(configBytes, &externalConfig); err != nil {
			return trace.Wrap(err, "failed to unmarshal kubelet configuration from JSON")
		}
		err = mergo.Merge(&kubeletConfig, externalConfig, mergo.WithOverride)
		if err != nil {
			return trace.Wrap(err)
		}
		err = applyConfigOverrides(&kubeletConfig)
		if err != nil {
			return trace.Wrap(err)
		}
	}
	configBytes, err := yaml.Marshal(kubeletConfig)
	if err != nil {
		return trace.Wrap(err)
	}
	config.Files = append(config.Files, box.File{
		Path:     constants.KubeletConfigFile,
		Contents: bytes.NewReader(configBytes),
		Mode:     SharedReadWriteMask,
	})
	return nil
}

func applyConfigOverrides(config *kubeletconfig.KubeletConfiguration) error {
	// Reset the attributes we expect to be set to specific values
	err := mergo.Merge(config, KubeletConfigOverrides, mergo.WithOverride)
	if err != nil {
		return trace.Wrap(err)
	}
	// Unfortunately, the read-only port is not modeled to distinguish between
	// specified and zero value, so force it to clear
	config.ReadOnlyPort = 0
	return nil
}

// addKubeConfig writes a kubectl config file
func addKubeConfig(config *Config) error {
	// Generate two kubectl configuration files: one will be used by
	// kubectl when invoked from host, another one - from planet,
	// because state directory may be different.
	kubeConfigs := map[string]string{
		constants.KubectlConfigPath:     constants.GravityDataDir,
		constants.KubectlHostConfigPath: config.HostStateDir(),
	}
	for configPath, stateDir := range kubeConfigs {
		kubeConfig, err := NewKubeConfig(config.APIServerIP(), stateDir)
		if err != nil {
			return trace.Wrap(err)
		}
		path := filepath.Join(config.Rootfs, configPath)
		err = os.MkdirAll(filepath.Dir(path), constants.SharedDirMask)
		if err != nil {
			return trace.Wrap(err)
		}
		// set read-only permissions for kubectl.kubeconfig to avoid annoying warning from Helm 3
		// 'WARNING: Kubernetes configuration file is group-readable. This is insecure. Location: /etc/kubernetes/kubectl.kubeconfig'
		err = utils.SafeWriteFile(path, kubeConfig, constants.OwnerReadMask)
		if err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

// setKubeConfigOwnership adjusts ownership of k8s config files to root:root
func setKubeConfigOwnership(config *Config) error {
	var errors []error
	for _, c := range []string{constants.SchedulerConfigPath, constants.ProxyConfigPath, constants.KubeletConfigPath} {
		err := os.Chown(filepath.Join(config.Rootfs, c), RootUID, RootGID)
		if err != nil {
			errors = append(errors, trace.ConvertSystemError(err))
		}
	}
	return trace.NewAggregate(errors...)
}

// setCoreDNS generates CoreDNS configuration for this server
func setCoreDNS(config *Config) error {
	resolv, err := readHostResolv()
	if err != nil {
		return trace.Wrap(err)
	}

	corednsConfig, err := generateCoreDNSConfig(coreDNSConfig{
		Zones:               config.DNS.Zones,
		Hosts:               config.DNS.Hosts,
		ListenAddrs:         config.DNS.ListenAddrs,
		Port:                config.DNS.Port,
		UpstreamNameservers: resolv.Servers,
		Rotate:              resolv.Rotate,
		Import:              true,
	}, coreDNSTemplate)
	if err != nil {
		return trace.Wrap(err)
	}

	err = ioutil.WriteFile(filepath.Join(config.Rootfs, CoreDNSConf), []byte(corednsConfig), constants.SharedReadMask)
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

func generateCoreDNSConfig(config coreDNSConfig, tpl string) (string, error) {
	parsed, err := template.New("coredns").Parse(tpl)
	if err != nil {
		return "", trace.Wrap(err)
	}

	var coredns bytes.Buffer
	err = parsed.Execute(&coredns, config)
	if err != nil {
		return "", trace.Wrap(err)
	}
	return coredns.String(), nil
}

type coreDNSConfig struct {
	Zones               map[string][]string
	Hosts               map[string][]string
	ListenAddrs         []string
	Port                int
	UpstreamNameservers []string
	Rotate              bool
	Import              bool
}

var coreDNSTemplate = `
.:{{.Port}} {
  reload
  bind{{range $bind := .ListenAddrs}} {{$bind}}{{- end}}
  errors
  hosts /etc/coredns/coredns.hosts {
    {{- range $hostname, $ips := .Hosts}}{{range $ip := $ips}}
    {{$ip}} {{$hostname}}{{end}}{{end}}
    fallthrough
  }
  kubernetes cluster.local in-addr.arpa ip6.arpa {
    endpoint https://leader.telekube.local:6443
    tls /var/state/coredns.cert /var/state/coredns.key /var/state/root.cert
    pods verified
    fallthrough in-addr.arpa ip6.arpa
  }{{range $zone, $servers := .Zones}}
  proxy {{$zone}} {{range $server := $servers}}{{$server}} {{end}}{
    policy sequential
  }{{end}}
  {{if .UpstreamNameservers}}forward . {{range $server := .UpstreamNameservers}}{{$server}} {{end}}{
    {{if .Rotate}}policy random{{else}}policy sequential{{end}}
    health_check 5s
  }{{end}}
}
`

func addResolv(config *Config) (upstreamNameservers []string, err error) {
	cfg, err := readHostResolv()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	planetResolv := config.inRootfs("etc", PlanetResolv)
	var dnsAddrs []string
	if len(config.DNS.ListenAddrs) != 0 {
		// Use the first configured listen address for planet
		// DNS resolution
		dnsAddrs = config.DNS.ListenAddrs[:1]
	}
	if err := copyResolvFile(*cfg, planetResolv, dnsAddrs); err != nil {
		return nil, trace.Wrap(err)
	}

	config.Mounts = append(config.Mounts, box.Mount{
		Src:      planetResolv,
		Dst:      "/etc/resolv.conf",
		Readonly: true,
	})

	return cfg.Servers, nil
}

func readHostResolv() (*utils.DNSConfig, error) {
	path, err := filepath.EvalSymlinks("/etc/resolv.conf")
	if err != nil {
		if os.IsNotExist(err) {
			return &utils.DNSConfig{}, nil
		}
		return nil, trace.Wrap(err)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	defer f.Close()
	cfg, err := utils.DNSReadConfig(f, true)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return cfg, nil
}

// copyResolvFile adds DNS resolver configuration from the host's /etc/resolv.conf
func copyResolvFile(cfg utils.DNSConfig, destination string, upstreamNameservers []string) error {
	// Make sure upstream nameservers go first in the order supplied by caller
	nameservers := append(upstreamNameservers, cfg.Servers...)

	cfg.Servers = nameservers
	cfg.Ndots = DNSNdots
	cfg.Timeout = DNSTimeout
	// Don't copy rotate option, we rely on query order for internal resolution
	cfg.Rotate = false

	resolv, err := os.OpenFile(
		destination,
		os.O_RDWR|os.O_CREATE|os.O_TRUNC, constants.SharedReadMask,
	)
	if err != nil {
		return trace.Wrap(err)
	}
	defer resolv.Close()

	_, err = io.WriteString(resolv, cfg.String())
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

func generateHosts() []utils.HostEntry {
	hosts := []utils.HostEntry{
		{IP: "127.0.0.1", Hostnames: "localhost localhost.localdomain localhost4 localhost4.localdomain4"},
		{IP: "::1", Hostnames: "localhost localhost.localdomain localhost6 localhost6.localdomain6"},
	}

	if utils.OnGCEVM() {
		hosts = append(hosts, utils.HostEntry{
			IP:        "169.254.169.254",
			Hostnames: "metadata.google.internal metadata",
		})
	}

	return hosts
}

func setHosts(config *Config, entries []utils.HostEntry) error {
	out := &bytes.Buffer{}
	if err := utils.WriteHosts(out, entries); err != nil {
		return trace.Wrap(err)
	}
	config.Files = append(config.Files, box.File{
		Path:     HostsFile,
		Contents: out,
		Mode:     constants.SharedReadWriteMask,
	})
	return nil
}

const (
	// CertificateAuthorityKeyPair is the name of the TLS cert authority
	// file (with .cert extension) that is used to sign APIserver
	// certificates and secret keys
	CertificateAuthorityKeyPair = "root"
	// APIServerKeyPair is the name of the apiserver keypair
	APIServerKeyPair = "apiserver"
	// RoleMaster sets up node as a K8s master server
	RoleMaster = "master"
	// RoleNode sets up planet as K8s node server
	RoleNode = "node"
)

// mountSecrets mounts files in secret directory under the specified
// location inside container
func mountSecrets(config *Config) {
	config.Mounts = append(config.Mounts, []box.Mount{
		{
			Src:      config.SecretsDir,
			Dst:      DefaultSecretsMountDir,
			Readonly: true,
		},
	}...)
}

func setupFlannel(config *Config) error {
	_ = os.Remove(path.Join(config.Rootfs, "/lib/systemd/system/multi-user.target.wants/flanneld.service"))

	config.Env.Upsert("KUBE_ENABLE_IPAM", "false")
	if config.DisableFlannel {
		// Historically we use etcd for IPAM when running flannel
		// In this case, we don't want to run the kubernetes IPAM, as the NodeSpec.PodCIDR may not match the flannel IPAM
		// Other plugins may want the kubernetes IPAM enabled, but unless we pass configuration we won't know.
		// For now, if flannel is disabled, assume the kubernetes IPAM should be enabled.
		config.Env.Upsert("KUBE_ENABLE_IPAM", "true")
		return nil
	}

	switch config.CloudProvider {
	case constants.CloudProviderAWS:
		config.Env.Upsert("FLANNEL_BACKEND", "aws-vpc")
	case constants.CloudProviderGCE:
		config.Env.Upsert("FLANNEL_BACKEND", "gce")
	default:
		config.Env.Upsert("FLANNEL_BACKEND", "vxlan")
	}

	err := os.Symlink(
		"/lib/systemd/system/flanneld.service",
		path.Join(config.Rootfs, "/lib/systemd/system/multi-user.target.wants/flanneld.service"),
	)
	if err != nil && !os.IsExist(err) {
		return trace.ConvertSystemError(err)
	}

	err = utils.SafeWriteFile(
		path.Join(config.Rootfs, "/etc/cni/net.d/10-flannel.conflist"),
		[]byte(flannelConflist),
		constants.SharedReadMask,
	)
	if err != nil {
		return trace.Wrap(err)
	}
	return nil

}

var flannelConflist = `
{
	"name": "cbr0",
	"cniVersion": "0.3.1",
	"plugins": [
	  {
		"type": "flannel",
		"delegate": {
		  "isDefaultGateway": true,
		  "hairpinMode": true
		}
	  },
	  {
		"type": "portmap",
		"capabilities": {
		  "portMappings": true
		}
	  }
	]
}
`

const (
	ETCDWorkDir              = "/ext/etcd"
	ETCDProxyDir             = "/ext/etcd/proxy"
	DockerWorkDir            = "/ext/docker"
	RegistryWorkDir          = "/ext/registry"
	ContainerEnvironmentFile = "/etc/container-environment"
)

func checkRequiredMounts(cfg *Config) error {
	expected := map[string]bool{
		ETCDWorkDir:     false,
		DockerWorkDir:   false,
		RegistryWorkDir: false,
	}
	for _, m := range cfg.Mounts {
		dst := filepath.Clean(m.Dst)
		if _, ok := expected[dst]; ok {
			expected[dst] = true
		}
		if dst == ETCDWorkDir {
			// remove the latest symlink, as it won't point to a valid path during chown below
			// and will get recreated during etcd initialization
			// if the file doesn't exist or we fail, it doesn't really matter if later steps are working
			_ = os.Remove(path.Join(m.Src, "latest"))

			// chown <service user>:<service group> /ext/etcd -r
			if err := chownDir(m.Src, cfg.ServiceUser.Uid, cfg.ServiceUser.Gid); err != nil {
				return trace.Wrap(err)
			}
		}
		if dst == DockerWorkDir {
			if ok, _ := check.IsBtrfsVolume(m.Src); ok {
				cfg.DockerBackend = "btrfs"
				log.Infof("Docker working directory is on BTRFS volume %q.", m.Src)
			}
		}
	}
	for k, v := range expected {
		if !v {
			return trace.BadParameter(
				"please supply mount source for data directory %q", k)
		}
	}
	return nil
}

// chownDir recursively chowns a directory and everything inside to
// a given uid:gid.
// It is a Golang equivalent of chown uid:gid dirPath -R
func chownDir(dirPath string, uid, gid int) error {
	if err := os.Chown(dirPath, uid, gid); err != nil {
		return err
	}
	return filepath.Walk(dirPath, func(path string, fi os.FileInfo, err error) error {
		return os.Chown(path, uid, gid)
	})
}

var (
	// awsCloudConfig is the cloud-config for integration with AWS
	awsCloudConfigTemplate = template.Must(template.New("aws").Parse(`[Global]
KubernetesClusterTag={{.ClusterID}}
`))
	// gceCloudConfig is the cloud-config for integration with GCE
	gceCloudConfigTemplate = template.Must(template.New("google").Parse(`[global]
; list of network tags on instances which will be used
; when creating firewall rules for load balancers
node-tags={{if .GCENodeTags}}{{.GCENodeTags}}{{else}}{{.ClusterID}}{{end}}
; enable multi-zone setting, otherwise kube-controller-manager
; will not recognize nodes running in different zones
multizone=true
`))
)

func monitorUnits(box *box.Box, units ...string) {
	unitState := make(map[string]string, len(units))
	for _, unit := range units {
		unitState[unit] = ""
	}
	start := time.Now()
	var inactiveUnits []string
	for i := 0; i < 30; i++ {
		for _, unit := range units {
			status, err := getStatus(box, unit)
			if err != nil && !isProgramNotRunningError(err) {
				log.WithFields(log.Fields{
					log.ErrorKey: err,
					"service":    unit,
				}).Warn("Failed to query service status.")
			}
			unitState[unit] = status
		}

		out := &bytes.Buffer{}
		fmt.Fprintf(out, "%v", time.Since(start))
		for _, unit := range units {
			if state, _ := unitState[unit]; state == serviceActive {
				fmt.Fprintf(out, " %v \x1b[32m[OK]\x1b[0m", unit)
			} else {
				fmt.Fprintf(out, " %v [%v]", unit, state)
			}
		}
		fmt.Println("\n", out.String())
		inactiveUnits = getInactiveUnits(unitState)
		if len(inactiveUnits) == 0 {
			fmt.Println("All units are up.")
			return
		}
		time.Sleep(time.Second)
	}

	fmt.Println("Some units have not started:", inactiveUnits)
	fmt.Println("Run `planet enter` and check journalctl for details.")
}

func getInactiveUnits(units map[string]string) (inactive []string) {
	for name, state := range units {
		if state != serviceActive {
			inactive = append(inactive, name)
		}
	}
	return inactive
}

func unitNames(units map[string]string) []string {
	out := []string{}
	for unit := range units {
		out = append(out, unit)
	}
	sort.StringSlice(out).Sort()
	return out
}

func getStatus(b *box.Box, unit string) (status string, err error) {
	out, err := b.CombinedOutput(box.ProcessConfig{
		User: "root",
		Args: []string{
			"/bin/systemctl", "is-active",
			unit,
		},
	})
	if err == nil {
		return serviceActive, nil
	}
	return strings.TrimSpace(string(out)), trace.Wrap(err)
}

func isRoot() bool {
	return os.Geteuid() == 0
}

var masterUnits = []string{
	"etcd",
	"flanneld",
	"docker",
	"kube-apiserver",
	"kube-controller-manager",
	"kube-scheduler",
	"kube-proxy",
	"kube-kubelet",
}

var nodeUnits = []string{
	"flanneld",
	"docker",
	"kube-proxy",
	"kube-kubelet",
	"etcd",
}

func isProgramNotRunningError(err error) bool {
	// See: http://refspecs.linuxbase.org/LSB_3.0.0/LSB-PDA/LSB-PDA/iniscrptact.html
	// LSB 3: program not running
	const exitProgramNotRunning = 3
	if status := utils.ExitStatusFromError(err); status != nil {
		return *status == exitProgramNotRunning
	}
	return false
}

const serviceActive = "active"
