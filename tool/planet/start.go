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

func startAndWait(config *Config) error {
	process, err := start(config, nil)

	if err != nil {
		return err
	}
	defer process.Close()

	// wait for the process to finish.
	status, err := process.Wait()
	if err != nil {
		return trace.Wrap(err)
	}
	log.Infof("box.Wait() returned status %v", status)
	return nil
}

func start(config *Config, monitorc chan<- bool) (*box.Box, error) {
	log.Infof("starting with config: %#v", config)

	if !isRoot() {
		return nil, trace.Errorf("must be run as root")
	}

	var err error

	// see if the kernel version is supported:
	if CheckKernel {
		v, err := check.KernelVersion()
		if err != nil {
			return nil, err
		}
		log.Infof("kernel: %v\n", v)
		if v < MinKernelVersion {
			err := trace.Errorf(
				"current minimum supported kernel version is %0.2f. Upgrade kernel before moving on.", MinKernelVersion/100.0)
			if !config.IgnoreChecks {
				return nil, err
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

	// check supported storage back-ends for docker
	config.DockerBackend, err = pickDockerStorageBackend()
	if err != nil {
		return nil, trace.Wrap(err)
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

	apiserverName := fmt.Sprintf("apiserver.%v", config.ClusterID)
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
		box.EnvPair{Name: EnvAPIServerName, Val: apiserverName},
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
		fmt.Sprintf("%v:5000", apiserverName),
	)

	addInsecureRegistries(config)
	addDockerOptions(config)
	setupFlannel(config)
	if err = setupCloudOptions(config); err != nil {
		return nil, err
	}
	if err = addResolv(config); err != nil {
		return nil, err
	}
	if config.hasRole(RoleMaster) {
		if err = mountSecrets(config); err != nil {
			return nil, err
		}
	}
	if err = setHosts(config, []HostEntry{
		HostEntry{IP: "127.0.0.1", Hostnames: "localhost localhost.localdomain localhost4 localhost4.localdomain4"},
		HostEntry{IP: "::1", Hostnames: "localhost localhost.localdomain localhost6 localhost6.localdomain6"},
		HostEntry{IP: config.MasterIP, Hostnames: fmt.Sprintf("apiserver.%v", config.ClusterID)},
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

	// start the container:
	b, err := box.Start(cfg)
	if err != nil {
		return nil, err
	}

	units := []string{}
	if config.hasRole(RoleMaster) {
		units = append(units, masterUnits...)
	}
	if config.hasRole(RoleNode) {
		units = appendUnique(units, nodeUnits)
	}

	go monitorUnits(b.Container, units, monitorc)

	return b, nil
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

	// check cloud provider settings
	if c.CloudProvider == "aws" {
		if c.Env.Get("AWS_ACCESS_KEY_ID") == "" || c.Env.Get("AWS_SECRET_ACCESS_KEY") == "" {
			return trace.Errorf("Cloud provider set to AWS, but AWS_KEY_ID and AWS_SECRET_ACCESS_KEY are not specified")
		}
	}

	flags := []string{fmt.Sprintf("--cloud-provider=%v", c.CloudProvider)}

	// generate AWS cloud config for kubernetes cluster
	if c.CloudProvider == "aws" && c.ClusterID != "" {
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
	err = trace.Errorf("none of the required filesystems are supported by this host: %s",
		strings.Join(supportedBackends, ", "))
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
	config.Mounts = append(config.Mounts, box.Mount{
		Src:      path,
		Dst:      "/etc/resolv.conf",
		Readonly: true,
	})
	return nil
}

func setHosts(config *Config, entries []HostEntry) error {
	out := &bytes.Buffer{}
	for _, e := range entries {
		io.WriteString(out, fmt.Sprintf("%v %v\n", e.IP, e.Hostnames))
	}
	config.Files = append(config.Files, box.File{
		Path:     "/etc/hosts",
		Contents: out,
		Mode:     0666,
	})
	return nil
}

type HostEntry struct {
	Hostnames string
	IP        string
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

// mountSecrets mounts k8s secrets directory
func mountSecrets(c *Config) error {
	p := &keyPairPaths{
		name:      CertificateAuthorityKeyPair,
		sourceDir: c.SecretsDir,
	}
	// key pair have been already initialized
	exists, err := p.exists()
	if err != nil {
		return trace.Wrap(err)
	}
	if !exists {
		return trace.Errorf("expected %v.cert file", CertificateAuthorityKeyPair)
	}

	p = &keyPairPaths{
		name:      APIServerKeyPair,
		sourceDir: c.SecretsDir,
	}
	// key pair have been already initialized
	exists, err = p.exists()
	if err != nil {
		return trace.Wrap(err)
	}
	if !exists {
		return trace.Errorf("expected %v.cert file", APIServerKeyPair)
	}

	c.Mounts = append(c.Mounts, []box.Mount{
		{
			Src:      c.SecretsDir,
			Dst:      "/var/state",
			Readonly: true,
		},
	}...)

	return nil
}

func setupFlannel(c *Config) {
	if c.CloudProvider == "aws" {
		c.Env.Upsert("FLANNEL_BACKEND", "aws-vpc")
	} else {
		c.Env.Upsert("FLANNEL_BACKEND", "vxlan")
	}
}

const (
	EtcdWorkDir              = "/ext/etcd"
	DockerWorkDir            = "/ext/docker"
	RegstrWorkDir            = "/ext/registry"
	ContainerEnvironmentFile = "/etc/container-environment"
)

func checkRequiredMounts(cfg *Config) error {
	expected := map[string]bool{
		EtcdWorkDir:   false,
		DockerWorkDir: false,
		RegstrWorkDir: false,
	}
	uid := atoi(cfg.ServiceUser.Uid)
	gid := atoi(cfg.ServiceUser.Gid)
	for _, m := range cfg.Mounts {
		dst := filepath.Clean(m.Dst)
		if _, ok := expected[dst]; ok {
			expected[dst] = true
		}
		if dst == EtcdWorkDir {
			// chown <service user>:<service group> /ext/etcd -r
			if err := chownDir(m.Src, uid, gid); err != nil {
				return err
			}
		}
		if dst == DockerWorkDir {
			if ok, _ := check.IsBtrfsVolume(m.Src); ok == true {
				cfg.DockerBackend = "btrfs"
				log.Warningf("Docker work dir is on btrfs volume: %v", m.Src)
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
	"AUDIT_CONTROL",
	"AUDIT_WRITE",
	"BLOCK_SUSPEND",
	"CHOWN",
	"DAC_OVERRIDE",
	"DAC_READ_SEARCH",
	"FOWNER",
	"FSETID",
	"IPC_LOCK",
	"IPC_OWNER",
	"KILL",
	"LEASE",
	"LINUX_IMMUTABLE",
	"MAC_ADMIN",
	"MAC_OVERRIDE",
	"MKNOD",
	"NET_ADMIN",
	"NET_BIND_SERVICE",
	"NET_BROADCAST",
	"NET_RAW",
	"SETGID",
	"SETFCAP",
	"SETPCAP",
	"SETUID",
	"SYS_ADMIN",
	"SYS_BOOT",
	"SYS_CHROOT",
	"SYS_MODULE",
	"SYS_NICE",
	"SYS_PACCT",
	"SYS_PTRACE",
	"SYS_RAWIO",
	"SYS_RESOURCE",
	"SYS_TIME",
	"SYS_TTY_CONFIG",
	"SYSLOG",
	"WAKE_ALARM",
}
