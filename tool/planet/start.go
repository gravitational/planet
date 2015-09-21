package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/orbit/lib/check"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/opencontainers/runc/libcontainer"
	"github.com/gravitational/planet/lib/box"
)

func start(conf Config) error {
	log.Infof("starting with config: %#v", conf)

	v, err := check.KernelVersion()
	if err != nil {
		return err
	}
	log.Infof("kernel: %v\n", v)
	if v < 313 {
		err := trace.Errorf(
			"current minimum supported kernel version is 3.13. Upgrade kernel before moving on.")
		if !conf.IgnoreChecks {
			return err
		}
		log.Infof("warning: %v", err)
	}

	ok, err := check.SupportsAufs()
	if err != nil {
		return err
	}
	if !ok {
		return trace.Errorf("need aufs support on the machine")
	}

	if conf.hasRole("master") {
		if err := checkMounts(conf); err != nil {
			return err
		}
	}

	conf.Env = append(conf.Env,
		box.EnvPair{Name: "KUBE_MASTER_IP", Val: conf.MasterIP},
		box.EnvPair{Name: "KUBE_CLOUD_PROVIDER", Val: conf.CloudProvider})

	// Always trust local registry (for now)
	conf.InsecureRegistries = append(
		conf.InsecureRegistries, fmt.Sprintf("%v:5000", conf.MasterIP))

	addInsecureRegistries(&conf)
	setupFlannel(&conf)
	if err := setupCloudOptions(&conf); err != nil {
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
		InitEnv:      []string{"container=libcontainer"},
		Capabilities: allCaps,
	}
	defer log.Infof("start() is done!")

	// start the container:
	b, err := box.Start(cfg)
	defer b.Close()
	if err != nil {
		return err
	}

	units := []string{}
	if conf.hasRole("master") {
		units = append(units, masterUnits...)
	}
	if conf.hasRole("node") {
		units = appendUnique(units, nodeUnits)
	}

	// go monitorUnits(b.Container, units)

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

func setupFlannel(c *Config) {
	if c.CloudProvider == "aws" {
		c.Env.Upsert("FLANNEL_BACKEND", "aws-vpc")
	} else {
		c.Env.Upsert("FLANNEL_BACKEND", "vxlan")
	}
}

func checkMounts(cfg Config) error {
	expected := map[string]bool{
		"/ext/etcd":     false,
		"/ext/registry": false,
		"/ext/docker":   false,
	}
	for _, m := range cfg.Mounts {
		dst := filepath.Clean(m.Dst)
		if _, ok := expected[dst]; ok {
			expected[dst] = true
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
