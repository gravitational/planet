package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gravitational/planet/lib/constants"

	"github.com/gravitational/planet/lib/box"
	"github.com/gravitational/planet/lib/check"
	"github.com/gravitational/planet/lib/user"
	"github.com/gravitational/planet/lib/utils"
	"github.com/gravitational/trace"
	"github.com/opencontainers/runc/libcontainer"
	log "github.com/sirupsen/logrus"
)

const DefaultSearchDomain = "cluster.local"

const MinKernelVersion = 310
const (
	CheckKernel          = true
	CheckCgroupMounts    = true
	DefaultServiceSubnet = "10.100.0.0/16"
	DefaultPODSubnet     = "10.244.0.0/16"
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
	ctx, err := start(config, nil)

	if err != nil {
		return err
	}
	defer ctx.Close()

	// wait for the process to finish.
	status, err := ctx.process.Wait()
	if err != nil {
		return trace.Wrap(err)
	}
	log.Infof("box.Wait() returned status %v", status)
	return nil
}

func start(config *Config, monitorc chan<- bool) (*runtimeContext, error) {
	log.Infof("starting with config: %#v", config)

	if !isRoot() {
		return nil, trace.Errorf("must be run as root")
	}

	var err error

	// see if the kernel version is supported:
	if CheckKernel {
		v, err := check.KernelVersion()
		if err != nil {
			return nil, trace.Wrap(err)
		}
		log.Infof("kernel: %v\n", v)
		if v < MinKernelVersion {
			err := trace.Errorf(
				"current minimum supported kernel version is %0.2f. Upgrade kernel before moving on.", MinKernelVersion/100.0)
			if !config.IgnoreChecks {
				return nil, trace.Wrap(err)
			}
			log.Infof("warning: %v", err)
		}
	}

	// check & mount cgroups:
	if CheckCgroupMounts {
		if err = box.MountCgroups("/"); err != nil {
			return nil, trace.Wrap(err)
		}
	}

	if config.DockerBackend == "" {
		// check supported storage back-ends for docker
		config.DockerBackend, err = pickDockerStorageBackend()
		if err != nil {
			return nil, trace.Wrap(err)
		}
	}

	// check/create user accounts and set permissions
	if err = ensureUsersGroups(config); err != nil {
		return nil, trace.Wrap(err)
	}

	if err = addUserToContainer(config.Rootfs); err != nil {
		return nil, trace.Wrap(err)
	}

	if err = addGroupToContainer(config.Rootfs); err != nil {
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
		box.EnvPair{Name: EnvMasterIP, Val: config.MasterIP},
		box.EnvPair{Name: EnvCloudProvider, Val: config.CloudProvider},
		box.EnvPair{Name: EnvServiceSubnet, Val: config.ServiceSubnet.String()},
		box.EnvPair{Name: EnvPODSubnet, Val: config.PODSubnet.String()},
		box.EnvPair{Name: EnvPublicIP, Val: config.PublicIP},
		// Default agent name to the name of the etcd member
		box.EnvPair{Name: EnvAgentName, Val: config.EtcdMemberName},
		box.EnvPair{Name: EnvInitialCluster, Val: toKeyValueList(config.InitialCluster)},
		box.EnvPair{Name: EnvClusterDNSIP, Val: config.SkyDNSResolverIP()},
		box.EnvPair{Name: EnvAPIServerName, Val: APIServerDNSName},
		box.EnvPair{Name: EnvEtcdProxy, Val: config.EtcdProxy},
		box.EnvPair{Name: EnvEtcdMemberName, Val: config.EtcdMemberName},
		box.EnvPair{Name: EnvEtcdInitialCluster, Val: config.EtcdInitialCluster},
		box.EnvPair{Name: EnvEtcdInitialClusterState, Val: config.EtcdInitialClusterState},
		box.EnvPair{Name: EnvRole, Val: config.Roles[0]},
		box.EnvPair{Name: EnvClusterID, Val: config.ClusterID},
		box.EnvPair{Name: EnvNodeName, Val: config.NodeName},
		box.EnvPair{Name: EnvElectionEnabled, Val: strconv.FormatBool(config.ElectionEnabled)},
	)

	addInsecureRegistries(config)
	addDockerOptions(config)
	setupFlannel(config)
	if err = setupCloudOptions(config); err != nil {
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

	if err = setDNSMasq(config); err != nil {
		return nil, trace.Wrap(err)
	}

	if err = addKubeConfig(config); err != nil {
		return nil, trace.Wrap(err)
	}

	if err = setKubeConfigOwnership(config); err != nil {
		return nil, trace.Wrap(err)
	}
	mountSecrets(config)

	err = setHosts(config, []utils.HostEntry{
		{IP: "127.0.0.1", Hostnames: "localhost localhost.localdomain localhost4 localhost4.localdomain4"},
		{IP: "::1", Hostnames: "localhost localhost.localdomain localhost6 localhost6.localdomain6"},
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}

	if config.Hostname != "" {
		// Set hostname
		config.Files = append(config.Files, box.File{
			Path:     HostnameFile,
			Contents: strings.NewReader(config.Hostname),
			Mode:     SharedReadWriteMask,
		})
	}

	cfg := box.Config{
		Rootfs:     config.Rootfs,
		SocketPath: config.SocketPath,
		EnvFiles: []box.EnvFile{
			box.EnvFile{
				Path: ContainerEnvironmentFile,
				Env:  config.Env,
			},
		},
		Files:        config.Files,
		Mounts:       config.Mounts,
		DataDir:      "/var/run/planet",
		InitUser:     "root",
		InitArgs:     []string{"/bin/systemd"},
		InitEnv:      []string{"container=libcontainer", "LC_ALL=en_US.UTF-8"},
		Capabilities: allCaps,
	}
	defer log.Infof("start() is done!")

	listener, err := newUdevListener(config.Rootfs, config.SocketPath)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// start the container:
	box, err := box.Start(cfg)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	units := []string{}
	if config.hasRole(RoleMaster) {
		units = append(units, masterUnits...)
	}
	if config.hasRole(RoleNode) {
		units = appendUnique(units, nodeUnits)
	}

	go monitorUnits(box.Container, units, monitorc)

	return &runtimeContext{
		process:  box,
		listener: listener,
	}, nil
}

// ensureUsersGroups ensures that all user accounts exist.
// If an account has not been created - it will attempt to create one.
func ensureUsersGroups(config *Config) error {
	var err error
	if config.ServiceGID == "" {
		config.ServiceGID = config.ServiceUID
	}
	config.ServiceUser, err = check.CheckUserGroup(check.PlanetUser, check.PlanetGroup, config.ServiceUID, config.ServiceGID)
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// addUserToContainer adds a record for planet user from host's passwd file
// into container's /etc/passwd.
func addUserToContainer(rootfs string) error {
	newSysFile := func(r io.Reader) (user.SysFile, error) {
		return user.NewPasswd(r)
	}
	hostPath := "/etc/passwd"
	containerPath := filepath.Join(rootfs, "etc", "passwd")
	rewrite := func(host, container user.SysFile) error {
		hostFile, _ := host.(user.PasswdFile)
		containerFile, _ := container.(user.PasswdFile)

		user, exists := hostFile.Get(check.PlanetUser)
		if !exists {
			return trace.Errorf("planet user not in host's passwd file")
		}
		log.Infof("adding user %v to container", user.Name)
		containerFile.Upsert(user)

		return nil
	}

	return upsertFromHost(hostPath, containerPath, newSysFile, rewrite)
}

// addGroupToContainer adds a record for planet group from host's group file
// into container's /etc/group.
func addGroupToContainer(rootfs string) error {
	newSysFile := func(r io.Reader) (user.SysFile, error) {
		return user.NewGroup(r)
	}
	hostPath := "/etc/group"
	containerPath := filepath.Join(rootfs, "etc", "group")
	rewrite := func(host, container user.SysFile) error {
		hostFile, _ := host.(user.GroupFile)
		containerFile, _ := container.(user.GroupFile)

		group, exists := hostFile.Get(check.PlanetGroup)
		if !exists {
			return trace.Errorf("planet group not in host's group file")
		}
		log.Infof("adding group %v to container", group.Name)
		containerFile.Upsert(group)

		return nil
	}

	return upsertFromHost(hostPath, containerPath, newSysFile, rewrite)
}

func upsertFromHost(hostPath, containerPath string, sysFile func(io.Reader) (user.SysFile, error),
	rewrite func(host, container user.SysFile) error) error {
	hostFile, err := os.Open(hostPath)
	if err != nil {
		return trace.Wrap(err)
	}
	hostSysFile, err := sysFile(hostFile)
	hostFile.Close()
	if err != nil {
		return trace.Wrap(err)
	}

	containerFile, err := os.Open(containerPath)
	if err != nil {
		return trace.Wrap(err)
	}

	containerSysFile, err := sysFile(containerFile)
	containerFile.Close()
	if err != nil {
		return trace.Wrap(err)
	}

	err = rewrite(hostSysFile, containerSysFile)
	if err != nil {
		return trace.Wrap(err)
	}

	containerFile, err = os.OpenFile(containerPath, os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return trace.Wrap(err)
	}
	err = containerSysFile.Save(containerFile)
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// setupCloudOptions sets up cloud flags and files passed to kubernetes
// binaries, sets up container environment files
func setupCloudOptions(c *Config) error {
	if c.CloudProvider == "" {
		return nil
	}

	flags := []string{fmt.Sprintf("--cloud-provider=%v", c.CloudProvider)}

	// generate AWS cloud config for kubernetes cluster
	if c.CloudProvider == CloudProviderAWS && c.ClusterID != "" {
		flags = append(flags,
			"--cloud-config=/etc/cloud-config.conf")
		c.Files = append(c.Files, box.File{
			Path: "/etc/cloud-config.conf",
			Contents: strings.NewReader(
				fmt.Sprintf(awsCloudConfig, c.ClusterID)),
		})
	}

	c.Env.Upsert("KUBE_CLOUD_FLAGS", strings.Join(flags, " "))

	return nil
}

func addInsecureRegistries(c *Config) {
	if len(c.InsecureRegistries) == 0 {
		return
	}
	out := make([]string, len(c.InsecureRegistries))
	for i, r := range c.InsecureRegistries {
		out[i] = fmt.Sprintf("--insecure-registry=%v", r)
	}
	opts := strings.Join(out, " ")
	c.Env.Append("DOCKER_OPTS", opts, " ")
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
func addDockerOptions(config *Config) {
	// add supported storage backend
	config.Env.Append("DOCKER_OPTS",
		fmt.Sprintf("--storage-driver=%s", config.DockerBackend), " ")

	// use cgroups native driver, because of this:
	// https://github.com/docker/docker/issues/16256
	config.Env.Append("DOCKER_OPTS", "--exec-opt native.cgroupdriver=cgroupfs", " ")
	// Add sensible size limits to logging driver
	config.Env.Append("DOCKER_OPTS", "--log-opt max-size=50m", " ")
	config.Env.Append("DOCKER_OPTS", "--log-opt max-file=9", " ")
	if config.DockerOptions != "" {
		config.Env.Append("DOCKER_OPTS", config.DockerOptions, " ")
	}
}

// addKubeConfig writes a kubectl config file
func addKubeConfig(config *Config) error {
	kubeConfig, err := NewKubeConfig(config.APIServerIP())
	if err != nil {
		return trace.Wrap(err)
	}
	path := filepath.Join(config.Rootfs, constants.KubectlConfigPath)
	err = os.MkdirAll(filepath.Dir(path), SharedDirMask)
	if err != nil {
		return trace.Wrap(err)
	}
	err = ioutil.WriteFile(path, kubeConfig, SharedFileMask)
	if err != nil {
		return trace.Wrap(err)
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

func setDNSMasq(config *Config) error {
	resolv, err := readHostResolv()
	if err != nil {
		return trace.Wrap(err)
	}
	out := &bytes.Buffer{}
	// Do not use local resolver
	fmt.Fprintf(out, "no-resolv\n")
	// Never forward plain names (without a dot or domain part)
	fmt.Fprintf(out, "domain-needed\n")
	// Listen on local interfaces, it's important to set those,
	// otherwise you hit this bug:
	// https://bugs.launchpad.net/ubuntu/+source/dnsmasq/+bug/1414887
	fmt.Fprintf(out, "listen-address=127.0.0.1\n")
	fmt.Fprintf(out, "listen-address=%v\n", config.PublicIP)
	// Use SkyDNS K8s resolver for cluster local stuff
	for _, searchDomain := range K8sSearchDomains {
		fmt.Fprintf(out, "server=/%v/%v\n", searchDomain, config.SkyDNSResolverIP())
	}
	for domainName, addrIP := range config.DNSOverrides {
		fmt.Fprintf(out, "address=/%v/%v\n", domainName, addrIP)
	}
	// Use host DNS for everything else
	for _, hostNameserver := range resolv.Servers {
		fmt.Fprintf(out, "server=%v\n", hostNameserver)
	}
	// do not send local requests to upstream servers
	fmt.Fprintf(out, "local=/cluster.local/\n")

	err = ioutil.WriteFile(filepath.Join(config.Rootfs, DNSMasqK8sConf), out.Bytes(), SharedFileMask)
	if err != nil {
		return trace.Wrap(err)
	}
	err = writeLocalLeader(filepath.Join(config.Rootfs, DNSMasqAPIServerConf), config.MasterIP)
	return trace.Wrap(err)
}

func addResolv(config *Config) (upstreamNameservers []string, err error) {
	cfg, err := readHostResolv()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	planetResolv := config.inRootfs("etc", PlanetResolv)
	if err := copyResolvFile(*cfg, planetResolv, []string{"127.0.0.1"}); err != nil {
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
	cfg, err := utils.DNSReadConfig(f)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return cfg, nil
}

// copyResolvFile adds DNS resolver configuration from the host's /etc/resolv.conf
func copyResolvFile(cfg utils.DNSConfig, destination string, upstreamNameservers []string) error {
	// Make sure upstream nameservers go first in the order supplied by caller
	nameservers := append([]string{}, upstreamNameservers...)
	nameservers = append(nameservers, cfg.Servers...)

	cfg.Servers = nameservers
	// Limit search to local cluster domain
	cfg.Search = []string{DefaultSearchDomain}
	cfg.Ndots = DNSNdots
	cfg.Timeout = DNSTimeout

	resolv, err := os.OpenFile(
		destination,
		os.O_RDWR|os.O_CREATE|os.O_TRUNC, SharedFileMask,
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

func writeKubeDNSConfig(config Config, dnsConfig utils.DNSConfig) error {
	f, err := os.OpenFile(
		config.inRootfs("etc", "kubernetes", kubeDNSConfigMap),
		os.O_RDWR|os.O_CREATE|os.O_TRUNC, SharedFileMask,
	)
	if err != nil {
		return trace.Wrap(err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	err = enc.Encode(struct {
		StubDomains map[string][]string `json:"stubDomains"`
		Nameservers []string            `json:"upstreamNameservers"`
	}{
		StubDomains: map[string][]string{
			"cluster.local": []string{"127.0.0.1#10053"},
		},
		Nameservers: dnsConfig.Servers,
	})
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

func setHosts(config *Config, entries []utils.HostEntry) error {
	out := &bytes.Buffer{}
	if err := utils.WriteHosts(out, entries); err != nil {
		return trace.Wrap(err)
	}
	config.Files = append(config.Files, box.File{
		Path:     HostsFile,
		Contents: out,
		Mode:     SharedReadWriteMask,
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

func setupFlannel(config *Config) {
	if config.CloudProvider == CloudProviderAWS {
		config.Env.Upsert("FLANNEL_BACKEND", "aws-vpc")
	} else {
		config.Env.Upsert("FLANNEL_BACKEND", "vxlan")
	}
}

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
	uid := atoi(cfg.ServiceUser.Uid)
	gid := atoi(cfg.ServiceUser.Gid)
	for _, m := range cfg.Mounts {
		dst := filepath.Clean(m.Dst)
		if _, ok := expected[dst]; ok {
			expected[dst] = true
		}
		if dst == ETCDWorkDir {
			// chown <service user>:<service group> /ext/etcd -r
			if err := chownDir(m.Src, uid, gid); err != nil {
				return err
			}
		}
		if dst == DockerWorkDir {
			if ok, _ := check.IsBtrfsVolume(m.Src); ok {
				cfg.DockerBackend = "btrfs"
				log.Infof("docker working directory is on BTRFS volume `%v`", m.Src)
			}
		}
	}
	for k, v := range expected {
		if !v {
			return trace.Errorf(
				"please supply mount source for data directory '%v'", k)
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

const awsCloudConfig = `[Global]
KubernetesClusterTag=%v
`

var masterUnits = []string{
	"etcd",
	"flanneld",
	"docker",
	"kube-apiserver",
	"kube-controller-manager",
}

var nodeUnits = []string{
	"flanneld",
	"docker",
	"kube-proxy",
	"kube-kubelet",
}

func appendUnique(a, b []string) []string {
	as := make(map[string]bool, len(a))
	for _, i := range a {
		as[i] = true
	}
	for _, i := range b {
		if _, ok := as[i]; !ok {
			a = append(a, i)
		}
	}
	return a
}

func monitorUnits(c libcontainer.Container, units []string, monitorc chan<- bool) {
	if monitorc != nil {
		defer close(monitorc)
	}

	unitState := make(map[string]string, len(units))
	for _, unit := range units {
		unitState[unit] = ""
	}
	start := time.Now()
	var inactiveUnits []string
	for i := 0; i < 30; i++ {
		for _, unit := range units {
			status, err := getStatus(c, unit)
			if err != nil {
				log.Infof("error getting status: %v", err)
			}
			unitState[unit] = status
		}

		out := &bytes.Buffer{}
		fmt.Fprintf(out, "%v", time.Now().Sub(start))
		for _, unit := range units {
			if unitState[unit] != "" {
				fmt.Fprintf(out, " %v \x1b[32m[OK]\x1b[0m", unit)
			} else {
				fmt.Fprintf(out, " %v[  ]", unit)
			}
		}
		fmt.Printf("\r %v", out.String())
		inactiveUnits = getInactiveUnits(unitState)
		if len(inactiveUnits) == 0 {
			if monitorc != nil {
				monitorc <- true
			}
			fmt.Printf("\nall units are up\n")
			return
		}
		time.Sleep(time.Second)
	}

	fmt.Printf("\nsome units have not started: %q.\n Run `planet enter` and check journalctl for details\n", inactiveUnits)
}

func getInactiveUnits(units map[string]string) (inactive []string) {
	for name, state := range units {
		if state == "" {
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

func getStatus(c libcontainer.Container, unit string) (string, error) {
	out, err := box.CombinedOutput(
		c, box.ProcessConfig{
			User: "root",
			Args: []string{
				"/bin/systemctl", "is-active",
				fmt.Sprintf("%v.service", unit),
			}})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// quick & convenient way to convert strings to ints, but can only be used
// for cases when we are FOR SURE know those are ints. It panics on
// input that can't be parsed into an int.
func atoi(s string) int {
	i, err := strconv.ParseInt(s, 0, 0)
	if err != nil {
		panic(trace.Errorf("bad number `%s`: %v", s, err))
	}
	return int(i)
}

func isRoot() bool {
	return os.Geteuid() == 0
}

var allCaps = []string{
	"CAP_AUDIT_CONTROL",
	"CAP_AUDIT_WRITE",
	"CAP_BLOCK_SUSPEND",
	"CAP_CHOWN",
	"CAP_DAC_OVERRIDE",
	"CAP_DAC_READ_SEARCH",
	"CAP_FOWNER",
	"CAP_FSETID",
	"CAP_IPC_LOCK",
	"CAP_IPC_OWNER",
	"CAP_KILL",
	"CAP_LEASE",
	"CAP_LINUX_IMMUTABLE",
	"CAP_MAC_ADMIN",
	"CAP_MAC_OVERRIDE",
	"CAP_MKNOD",
	"CAP_NET_ADMIN",
	"CAP_NET_BIND_SERVICE",
	"CAP_NET_BROADCAST",
	"CAP_NET_RAW",
	"CAP_SETGID",
	"CAP_SETFCAP",
	"CAP_SETPCAP",
	"CAP_SETUID",
	"CAP_SYS_ADMIN",
	"CAP_SYS_BOOT",
	"CAP_SYS_CHROOT",
	"CAP_SYS_MODULE",
	"CAP_SYS_NICE",
	"CAP_SYS_PACCT",
	"CAP_SYS_PTRACE",
	"CAP_SYS_RAWIO",
	"CAP_SYS_RESOURCE",
	"CAP_SYS_TIME",
	"CAP_SYS_TTY_CONFIG",
	"CAP_SYSLOG",
	"CAP_WAKE_ALARM",
}
