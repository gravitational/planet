package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/orbit/box"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/orbit/check"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/opencontainers/runc/libcontainer"
)

func start(conf CubeConfig) error {
	log.Infof("starting with config: %#v", conf)

	v, err := check.KernelVersion()
	if err != nil {
		return err
	}
	log.Infof("kernel: %v\n", v)
	if v < 313 {
		err := trace.Errorf(
			"current minimum supported kernel version is 3.13. Upgrade kernel before moving on.")
		if !conf.Force {
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

	if err := checkMounts(conf); err != nil {
		return err
	}

	conf.Env = append(conf.Env,
		box.EnvPair{Name: "KUBE_MASTER_IP", Val: conf.MasterIP},
		box.EnvPair{Name: "KUBE_CLOUD_PROVIDER", Val: conf.CloudProvider},
		box.EnvPair{Name: "KUBE_CLOUD_CONFIG", Val: conf.CloudConfig})

	cfg := box.Config{
		Rootfs: conf.Rootfs,
		EnvFiles: []box.EnvFile{
			box.EnvFile{
				Path: "/etc/container-environment",
				Env:  conf.Env,
			},
		},
		Mounts:       conf.Mounts,
		DataDir:      "/var/run/cube",
		InitUser:     "root",
		InitArgs:     []string{"/bin/systemd"},
		InitEnv:      []string{"container=libcontainer"},
		Capabilities: allCaps,
	}

	b, err := box.Start(cfg)
	if err != nil {
		return err
	}

	if conf.hasRole("master") {
		go monitorMasterUnits(b.Container)
	}
	if conf.hasRole("node") {
		go monitorNodeUnits(b.Container)
	}

	// wait for the process to finish.
	status, err := b.Wait()
	if err != nil {
		return trace.Wrap(err)
	}

	log.Infof("process status: %v %v", status, err)
	return nil
}

func checkMounts(cfg CubeConfig) error {
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

func monitorMasterUnits(c libcontainer.Container) {
	units := map[string]string{
		"docker.service":                  "",
		"flanneld.service":                "",
		"etcd.service":                    "",
		"kube-apiserver.service":          "",
		"kube-controller-manager.service": "",
		"kube-scheduler.service":          "",
	}

	for i := 0; i < 10; i++ {
		for u := range units {
			status, err := getStatus(c, u)

			if err != nil {
				log.Infof("error getting status: %v", err)
			}
			units[u] = status
		}

		for u, s := range units {
			if s != "" {
				fmt.Printf("* %v[OK]\n", u)
				delete(units, u)
			}

		}
		if len(units) == 0 {
			fmt.Printf("[cube-master] all units are up\n")
			return
		} else {
			fmt.Printf("[cube-master] waiting for %v\n", unitNames(units))
		}
		time.Sleep(time.Second)
	}
}

func monitorNodeUnits(c libcontainer.Container) {
	units := map[string]string{
		"docker.service":       "",
		"flanneld.service":     "",
		"kube-proxy.service":   "",
		"kube-kubelet.service": "",
	}

	for i := 0; i < 10; i++ {
		for u := range units {
			status, err := getStatus(c, u)

			if err != nil {
				log.Infof("error getting status: %v", err)
			}
			units[u] = status
		}

		for u, s := range units {
			if s != "" {
				fmt.Printf("* %v[OK]\n", u)
				delete(units, u)
			}

		}
		if len(units) == 0 {
			fmt.Printf("[cube-node] all units are up\n")
			return
		} else {
			fmt.Printf("[cube-node] waiting for %v\n", unitNames(units))
		}
		time.Sleep(time.Second)
	}
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
			Args: []string{"/bin/systemctl", "is-active", unit}})
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
