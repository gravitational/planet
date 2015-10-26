package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/opencontainers/runc/libcontainer"
	"github.com/gravitational/planet/lib/box"
	"github.com/gravitational/planet/lib/check"
)

const MinKernelVersion = 313
const (
	CheckKernel       = true
	CheckCgroupMounts = true
)

func start(conf Config) (err error) {
	log.Infof("starting with config: %#v", conf)

	if !isRoot() {
		return trace.Errorf("must be run as root")
	}

	// see if the kernel version is supported:
	if CheckKernel {
		v, err := check.KernelVersion()
		if err != nil {
			return err
		}
		log.Infof("kernel: %v\n", v)
		if v < MinKernelVersion {
			err := trace.Errorf(
				"current minimum supported kernel version is %0.2f. Upgrade kernel before moving on.", MinKernelVersion/100.0)
			if !conf.IgnoreChecks {
				return err
			}
			log.Infof("warning: %v", err)
		}
	}

	// check & mount cgroups:
	if CheckCgroupMounts {
		if err := box.MountCgroups("/"); err != nil {
			return trace.Wrap(err)
		}
	}

	// check supported storage back-ends for docker
	conf.DockerBackend, err = pickDockerStorageBackend()
	if err != nil {
		return trace.Wrap(err)
	}

	// check/create 'planet' user and set the permissions
	conf.PlanetUser, err = check.CheckPlanetUser()
	if err != nil {
		return trace.Wrap(err)
	}

	// validate the mounts:
	if conf.hasRole("master") {
		if err := checkMasterMounts(&conf); err != nil {
			return trace.Wrap(err)
		}
	}

	if conf.hasRole("node") {
		if err := checkNodeMounts(&conf); err != nil {
			return trace.Wrap(err)
		}
	}

	conf.Env = append(conf.Env,
		box.EnvPair{Name: "KUBE_MASTER_IP", Val: conf.MasterIP},
		box.EnvPair{Name: "KUBE_CLOUD_PROVIDER", Val: conf.CloudProvider},
		box.EnvPair{Name: "KUBE_SERVICE_SUBNET", Val: conf.ServiceSubnet.String()},
		box.EnvPair{Name: "KUBE_POD_SUBNET", Val: conf.PODSubnet.String()},
	)

	// Always trust local registry (for now)
	conf.InsecureRegistries = append(
		conf.InsecureRegistries, fmt.Sprintf("%v:5000", conf.MasterIP))

	addInsecureRegistries(&conf)
	addDockerStorage(&conf)
	setupFlannel(&conf)
	if err := setupCloudOptions(&conf); err != nil {
		return err
	}
	if err := addResolv(&conf); err != nil {
		return err
	}
	if err := initState(&conf); err != nil {
		return err
	}

	cfg := box.Config{
		Rootfs: conf.Rootfs,
		EnvFiles: []box.EnvFile{
			box.EnvFile{
				Path: "/etc/container-environment",
				Env:  conf.Env,
			},
		},
		Files:        conf.Files,
		Mounts:       conf.Mounts,
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
		return err
	}
	defer b.Close()

	units := []string{}
	if conf.hasRole("master") {
		units = append(units, masterUnits...)
	}
	if conf.hasRole("node") {
		units = appendUnique(units, nodeUnits)
	}

	go monitorUnits(b.Container, units)

	// wait for the process to finish.
	status, err := b.Wait()
	if err != nil {
		return trace.Wrap(err)
	}
	log.Infof("box.Wait() returned status %v", status)
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
	if dopts := c.Env.Get("DOCKER_OPTS"); dopts != "" {
		c.Env.Upsert("DOCKER_OPTS", strings.Join([]string{dopts, opts}, " "))
	} else {
		c.Env.Upsert("DOCKER_OPTS", opts)
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
	err = trace.Errorf("none of the required filesystems are supported by this host: %s",
		strings.Join(supportedBackends, ", "))
	return "", err
}

// addDockerStorage adds a given docker storage back-end to DOCKER_OPTS environment
// variable
func addDockerStorage(c *Config) {
	c.Env.Upsert("DOCKER_OPTS",
		fmt.Sprintf("%s --storage-driver=%s", c.Env.Get("DOCKER_OPTS"), c.DockerBackend))

	log.Infof("DOCKER_OPTS are: %v", c.Env.Get("DOCKER_OPTS"))
}

// addResolv adds resolv conf from the host's /etc/resolv.conf
func addResolv(c *Config) error {
	path, err := filepath.EvalSymlinks("/etc/resolv.conf")
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return trace.Wrap(err)
	}
	c.Mounts = append(c.Mounts, box.Mount{
		Src:      path,
		Dst:      "/etc/resolv.conf",
		Readonly: true,
	})
	return nil
}

// initState makes sure that k8s specific state like x509 keys and certs
// is initialized, it makes sure it's set up after it returns
// TODO(klizhentas) Make sure that worker nodes can use the same CA
// and APIserver actually checks the client certs.
// TODO(klizhentas) CA private key should not really be present on the
// master/nodes should be stored somewhere else
func initState(c *Config) error {
	if err := os.MkdirAll(c.StateDir, 0777); err != nil {
		return trace.Wrap(err)
	}

	// init key pair for certificate authority
	ca, err := initKeyPair(c, "root", nil, true)
	if err != nil {
		return err
	}

	// init key pair for apiserver signed by our authority
	_, err = initKeyPair(c, "apiserver", ca.keyPair, false)
	if err != nil {
		return err
	}

	c.Mounts = append(c.Mounts, []box.Mount{
		{
			Src:      c.StateDir,
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
	EtcdWorkDir   = "/ext/etcd"
	DockerWorkDir = "/ext/docker"
	RegstrWorkDir = "/ext/registry"
)

func checkMasterMounts(cfg *Config) error {
	expected := map[string]bool{
		EtcdWorkDir:   false,
		DockerWorkDir: false,
		RegstrWorkDir: false,
	}
	for _, m := range cfg.Mounts {
		dst := filepath.Clean(m.Dst)
		if _, ok := expected[dst]; ok {
			expected[dst] = true
		}
		if dst == EtcdWorkDir && cfg.hasRole("master") {
			uid := atoi(cfg.PlanetUser.Uid)
			gid := atoi(cfg.PlanetUser.Gid)
			// chown planet:planet /ext/etcd -r
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

// TODO: reduce code duplication with checkMasterMounts
func checkNodeMounts(cfg *Config) error {
	expected := map[string]bool{
		DockerWorkDir: false,
		RegstrWorkDir: false,
	}
	for _, m := range cfg.Mounts {
		dst := filepath.Clean(m.Dst)
		if _, ok := expected[dst]; ok {
			expected[dst] = true
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

func monitorUnits(c libcontainer.Container, units []string) {
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
