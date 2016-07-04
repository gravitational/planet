package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/gravitational/planet/lib/box"
	"github.com/gravitational/planet/lib/check"
	"github.com/gravitational/planet/lib/user"
	"github.com/gravitational/planet/lib/utils"
	"github.com/gravitational/trace"
	"github.com/opencontainers/runc/libcontainer"
)

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
	if err = ensureStateDir(config); err != nil {
		return nil, trace.Wrap(err)
	}

	config.Env = append(config.Env,
		box.EnvPair{Name: EnvMasterIP, Val: config.MasterIP},
		box.EnvPair{Name: EnvCloudProvider, Val: config.CloudProvider},
		box.EnvPair{Name: EnvServiceSubnet, Val: config.ServiceSubnet.String()},
		box.EnvPair{Name: EnvPODSubnet, Val: config.PODSubnet.String()},
		box.EnvPair{Name: EnvPublicIP, Val: config.PublicIP},
		box.EnvPair{Name: EnvStateDir, Val: config.StateDir},
		// Default agent name to the name of the etcd member
		box.EnvPair{Name: EnvAgentName, Val: config.EtcdMemberName},
		box.EnvPair{Name: EnvInitialCluster, Val: toKeyValueList(config.InitialCluster)},
		box.EnvPair{Name: EnvClusterDNSIP, Val: config.ServiceSubnet.RelativeIP(3).String()},
		box.EnvPair{Name: EnvAPIServerName, Val: APIServerDNSName},
		box.EnvPair{Name: EnvEtcdMemberName, Val: config.EtcdMemberName},
		box.EnvPair{Name: EnvEtcdInitialCluster, Val: config.EtcdInitialCluster},
		box.EnvPair{Name: EnvEtcdInitialClusterState, Val: config.EtcdInitialClusterState},
		box.EnvPair{Name: EnvRole, Val: config.Roles[0]},
		box.EnvPair{Name: EnvClusterID, Val: config.ClusterID},
		box.EnvPair{Name: EnvNodeName, Val: config.NodeName},
	)

	// Always trust local registry (for now)
	config.InsecureRegistries = append(
		config.InsecureRegistries,
		fmt.Sprintf("%v:5000", config.MasterIP),
		fmt.Sprintf("%v:5000", APIServerDNSName),
	)

	addInsecureRegistries(config)
	addDockerOptions(config)
	setupFlannel(config)
	if err = setupCloudOptions(config); err != nil {
		return nil, trace.Wrap(err)
	}
	if err = addResolv(config); err != nil {
		return nil, trace.Wrap(err)
	}
	if config.hasRole(RoleMaster) {
		if err = mountMasterSecrets(config); err != nil {
			return nil, trace.Wrap(err)
		}
	} else {
		mountSecrets(config)
	}
	if err = setHosts(config, []utils.HostEntry{
		{IP: "127.0.0.1", Hostnames: "localhost localhost.localdomain localhost4 localhost4.localdomain4"},
		{IP: "::1", Hostnames: "localhost localhost.localdomain localhost6 localhost6.localdomain6"},
		{IP: config.MasterIP, Hostnames: APIServerDNSName},
	}); err != nil {
		return nil, trace.Wrap(err)
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
	if c.DockerOptions != "" {
		c.Env.Append("DOCKER_OPTS", c.DockerOptions, " ")
	}
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
	err = trace.Errorf("none of the required filesystems are supported by this host: %v",
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
}

// addResolv adds resolv conf from the host's /etc/resolv.conf
func addResolv(config *Config) error {
	path, err := filepath.EvalSymlinks("/etc/resolv.conf")
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return trace.Wrap(err)
	}
	f, err := os.Open(path)
	if err != nil {
		return trace.Wrap(err)
	}
	defer f.Close()
	cfg, err := utils.DNSReadConfig(f)
	if err != nil {
		return trace.Wrap(err)
	}
	cfg.UpsertServer(config.PublicIP)
	cfg.Ndots = DNSNdots
	cfg.Timeout = DNSTimeout

	resolv, err := os.OpenFile(
		filepath.Join(config.Rootfs, "etc", "resolv.gravity.conf"),
		os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644,
	)
	defer resolv.Close()
	if err != nil {
		return trace.Wrap(err)
	}
	_, err = io.WriteString(resolv, cfg.String())
	if err != nil {
		return trace.Wrap(err)
	}
	config.Mounts = append(config.Mounts, box.Mount{
		Src:      resolv.Name(),
		Dst:      "/etc/resolv.conf",
		Readonly: true,
	})
	return nil
}

func setHosts(config *Config, entries []utils.HostEntry) error {
	hosts, err := os.Open("/etc/hosts")
	if err != nil {
		return trace.Wrap(err)
	}
	defer hosts.Close()
	out := &bytes.Buffer{}
	if err := utils.UpsertHostsLines(hosts, out, entries); err != nil {
		return trace.Wrap(err)
	}
	config.Files = append(config.Files, box.File{
		Path:     "/etc/hosts",
		Contents: out,
		Mode:     0666,
	})
	return nil
}

const (
	// CertificateAuthorityKeyPair is the name of the TLS cert authority
	// file (with .cert extension) that is used to sign APIserver
	// certificates and secret keys
	CertificateAuthorityKeyPair = "root"
	// APIServerKeyPair is the name
	APIServerKeyPair = "apiserver"
	// RoleMaster sets up node as a K8s master server
	RoleMaster = "master"
	// RoleNode sets up planet as K8s node server
	RoleNode = "node"
)

// mountMasterSecrets mounts k8s secrets directory
func mountMasterSecrets(config *Config) error {
	names := []string{CertificateAuthorityKeyPair, APIServerKeyPair}
	for _, name := range names {
		exists, err := validateKeyPair(config.SecretsDir, name)
		if err != nil {
			return trace.Wrap(err)
		}
		if !exists {
			return trace.Errorf("expected %v.cert file", name)
		}
	}

	mountSecrets(config)
	return nil
}

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

// validateKeyPair validates existence of certificate/key pair in dir
// using baseName as a base name for files
func validateKeyPair(dir, baseName string) (exists bool, err error) {
	path := func(fileType string) string {
		return filepath.Join(dir, fmt.Sprintf("%v.%v", baseName, fileType))
	}
	validatePath := func(path string) bool {
		if _, err = os.Stat(path); err != nil {
			err = trace.Wrap(err)
			return false
		}
		return true
	}
	keyPath := path("key")
	certPath := path("cert")
	haveKey := validatePath(keyPath)
	haveCert := validatePath(certPath)

	if err != nil {
		return false, trace.Wrap(err)
	}

	if !haveCert && haveKey {
		return false, trace.Errorf("cert `%v` is missing", certPath)
	}
	if haveCert && !haveKey {
		return false, trace.Errorf("key `%v` is missing", keyPath)
	}

	return haveCert && haveKey, nil
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

// configureMonitrcPermissions sets up proper file permissions on monit configuration file.
// monit places the following requirements on monitrc:
//  * it needs to be owned by the user used to spawn monit process (root)
//  * it requires 0700 permissions mask (u=rwx, go-rwx)
func configureMonitrcPermissions(rootfs string) error {
	const (
		monitrc = "/lib/monit/init/monitrc" // FIXME: this needs to be configurable
		rootUid = 0
		rootGid = 0
		rwxMask = 0700
	)
	var err error
	var rcpath string

	rcpath = filepath.Join(rootfs, monitrc)
	log.Infof("configuring permissions for `%s`", rcpath)
	err = os.Chmod(rcpath, rwxMask)
	if err != nil {
		return trace.Wrap(err)
	}
	err = os.Chown(rcpath, rootUid, rootGid)
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// ensureStateDir creates the directory for agent state
func ensureStateDir(config *Config) error {
	path := filepath.Join(config.Rootfs, config.StateDir)
	if err := os.MkdirAll(path, 0755); err != nil {
		return trace.Wrap(err, "failed to create state dir")
	}
	uid := atoi(config.ServiceUID)
	gid := atoi(config.ServiceGID)
	if err := os.Chown(path, uid, gid); err != nil {
		return trace.Wrap(err, "failed to chown state dir for service user/group")
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
	"kube-scheduler",
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

	us := make(map[string]string, len(units))
	for _, u := range units {
		us[u] = ""
	}
	start := time.Now()
	for i := 0; i < 30; i++ {
		for _, u := range units {
			status, err := getStatus(c, u)
			if err != nil {
				log.Infof("error getting status: %v", err)
			}
			us[u] = status
		}

		out := &bytes.Buffer{}
		fmt.Fprintf(out, "%v", time.Now().Sub(start))
		for _, u := range units {
			if us[u] != "" {
				fmt.Fprintf(out, " %v \x1b[32m[OK]\x1b[0m", u)
			} else {
				fmt.Fprintf(out, " %v[  ]", u)
			}
		}
		fmt.Printf("\r %v", out.String())
		if allUp(us) {
			if monitorc != nil {
				monitorc <- true
			}
			fmt.Printf("\nall units are up\n")
			return
		}
		time.Sleep(time.Second)
	}

	fmt.Printf("\nsome units have not started.\n Run `planet enter` and check journalctl for details\n")
}

func allUp(us map[string]string) bool {
	for _, v := range us {
		if v == "" {
			return false
		}
	}
	return true
}

func unitNames(units map[string]string) []string {
	out := []string{}
	for u := range units {
		out = append(out, u)
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
