package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gravitational/planet/lib/box"
	"github.com/gravitational/planet/lib/check"
	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/user"
	"github.com/gravitational/planet/lib/utils"

	"github.com/gravitational/trace"
	"github.com/opencontainers/runc/libcontainer"
	log "github.com/sirupsen/logrus"
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

	ctx, err := start(config, nil)
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

func start(config *Config, monitorc chan<- bool) (*runtimeContext, error) {
	log.Infof("starting with config: %#v", config)

	if !isRoot() {
		return nil, trace.Errorf("must be run as root")
	}

	var err error

	// see if the kernel version is supported:
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
	if err = addUserToContainer(config.Rootfs, config.ServiceUser.Uid); err != nil {
		return nil, trace.Wrap(err)
	}

	if err = addGroupToContainer(config.Rootfs, config.ServiceUser.Gid); err != nil {
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
		box.EnvPair{Name: EnvEtcdGatewayEndpoints, Val: config.EtcdGatewayList},
		box.EnvPair{Name: EnvEtcdInitialClusterState, Val: config.EtcdInitialClusterState},
		box.EnvPair{Name: EnvRole, Val: config.Roles[0]},
		box.EnvPair{Name: EnvClusterID, Val: config.ClusterID},
		box.EnvPair{Name: EnvNodeName, Val: config.NodeName},
		box.EnvPair{Name: EnvElectionEnabled, Val: strconv.FormatBool(config.ElectionEnabled)},
		box.EnvPair{Name: EnvDockerPromiscuousMode, Val: strconv.FormatBool(config.DockerPromiscuousMode)},
	)

	if err = addDockerOptions(config); err != nil {
		return nil, trace.Wrap(err)
	}
	addEtcdOptions(config)
	addKubeletOptions(config)
	setupFlannel(config)
	if err = setupCloudOptions(config); err != nil {
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
		InitEnv:      []string{"container=docker", "LC_ALL=en_US.UTF-8"},
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

// addUserToContainer adds a record for planet user from host's passwd file
// into container's /etc/passwd.
func addUserToContainer(rootfs string, id int) error {
	newSysFile := func(r io.Reader) (user.SysFile, error) {
		return user.NewPasswd(r)
	}
	rewrite := func(host, container user.SysFile) error {
		hostFile, _ := host.(user.PasswdFile)
		containerFile, _ := container.(user.PasswdFile)

		user, exists := hostFile.GetByID(id)
		if !exists {
			return trace.NotFound("user with UID %q not found on host", id)
		}
		log.Debugf("Adding user %+v to container.", user)
		user.Name = ServiceUser
		containerFile.Upsert(user)

		return nil
	}
	containerPath := filepath.Join(rootfs, UsersDatabase)
	err := upsertFromHost(UsersDatabase, containerPath, newSysFile, rewrite)
	if err != nil {
		err = upsertFromHost(UsersExtraDatabase, containerPath, newSysFile, rewrite)
	}
	return trace.Wrap(err)
}

// addGroupToContainer adds a record for planet group from host's group file
// into container's /etc/group.
func addGroupToContainer(rootfs string, id int) error {
	newSysFile := func(r io.Reader) (user.SysFile, error) {
		return user.NewGroup(r)
	}
	rewrite := func(host, container user.SysFile) error {
		hostFile, _ := host.(user.GroupFile)
		containerFile, _ := container.(user.GroupFile)

		group, exists := hostFile.GetByID(id)
		if !exists {
			return trace.NotFound("group with GID %q not found on host", id)
		}
		log.Debugf("Adding group %+v to container.", group)
		group.Name = ServiceGroup
		containerFile.Upsert(group)

		return nil
	}
	containerPath := filepath.Join(rootfs, GroupsDatabase)
	err := upsertFromHost(GroupsDatabase, containerPath, newSysFile, rewrite)
	if err != nil {
		err = upsertFromHost(GroupsExtraDatabase, containerPath, newSysFile, rewrite)
	}
	return trace.Wrap(err)
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
		fmt.Sprintf("--storage-driver=%s", config.DockerBackend), " ")

	// use cgroups native driver, because of this:
	// https://github.com/docker/docker/issues/16256
	config.Env.Append(EnvDockerOptions, "--exec-opt native.cgroupdriver=cgroupfs", " ")
	// Add sensible size limits to logging driver
	config.Env.Append(EnvDockerOptions, "--log-opt max-size=50m", " ")
	config.Env.Append(EnvDockerOptions, "--log-opt max-file=9", " ")
	if config.DockerOptions != "" {
		config.Env.Append(EnvDockerOptions, config.DockerOptions, " ")
	}

	if config.DockerPromiscuousMode {
		dropInDir := filepath.Join(config.Rootfs, constants.SystemdUnitPath, utils.DropInDir(DefaultDockerUnit))
		err := utils.WriteDropIn(dropInDir, DockerPromiscuousModeDropIn, []byte(`
[Service]
ExecStartPost=
ExecStartPost=/usr/bin/gravity system enable-promisc-mode docker0
ExecStopPost=
ExecStopPost=-/usr/bin/gravity system disable-promisc-mode docker0
`))
		if err != nil {
			return trace.Wrap(err)
		}
	}

	return nil
}

// addEtcdOptions sets extra etcd command line arguments in environment
func addEtcdOptions(config *Config) {
	if config.EtcdOptions != "" {
		config.Env.Append(EnvEtcdOptions, config.EtcdOptions, " ")
	}
}

// setupEtcd runs setup tasks for etcd
// If this is a proxy node, symlink in the etcd gateway dropin, so the etcd service runs the gateway and not etcd
// If this is a master node, and we don't detect an existing data directory, start the latest etcd, since we default
// to using the oldest etcd during an upgrade
func setupEtcd(config *Config) error {
	if strings.ToLower(config.EtcdProxy) == "on" {
		err := os.MkdirAll(path.Join(config.Rootfs, "etc/systemd/system/etcd.service.d/"), 0755)
		if err != nil {
			return trace.Wrap(err)
		}

		dropinPath := path.Join(config.Rootfs, "etc/systemd/system/etcd.service.d/10-gateway.conf")
		if _, err := os.Stat(dropinPath); os.IsNotExist(err) {
			err = os.Symlink(
				"/usr/lib/etcd/etcd-gateway.dropin",
				dropinPath,
			)
			if err != nil {
				return trace.Wrap(err)
			}
		}

	}
	return nil
}

// addKubeletOptions sets extra kubelet command line arguments in environment
func addKubeletOptions(config *Config) {
	if config.KubeletOptions != "" {
		config.Env.Append(EnvKubeletOptions, config.KubeletOptions, " ")
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
	fmt.Fprintf(out, "interface=lo\n")
	fmt.Fprintf(out, "bind-interfaces\n")
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
	nameservers := append(upstreamNameservers, cfg.Servers...)

	cfg.Servers = nameservers
	// Limit search to local cluster domain
	cfg.Search = nil
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
	for _, m := range cfg.Mounts {
		dst := filepath.Clean(m.Dst)
		if _, ok := expected[dst]; ok {
			expected[dst] = true
		}
		if dst == ETCDWorkDir {
			// chown <service user>:<service group> /ext/etcd -r
			if err := chownDir(m.Src, cfg.ServiceUser.Uid, cfg.ServiceUser.Gid); err != nil {
				return err
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
	"etcd",
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

func isRoot() bool {
	return os.Geteuid() == 0
}
