package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gravitational/trace"

	"code.google.com/p/go-uuid/uuid"
	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/configs"
)

func start(cfg CubeConfig) error {
	log.Printf("starting with config: %v", cfg)

	if os.Geteuid() != 0 {
		trace.Errorf("should be run as root")
	}
	k, m, err := parseKernel()
	if err != nil {
		return err
	}

	log.Printf("kernel: %v.%v\n", k, m)
	if k*100+m != 313 {
		err := trace.Errorf(
			"current supported kernel version is 3.13. Upgrade kernel before moving on.")
		if !cfg.Force {
			return err
		}
		log.Printf("warning: %v", err)
	}

	if err := supportsAufs(); err != nil {
		return err
	}

	rootfs, err := checkPath(cfg.Rootfs, false)
	if err != nil {
		return err
	}

	if err := mayBeMountCgroups("/"); err != nil {
		return err
	}

	log.Printf("starting container process in '%v'", rootfs)

	log.Printf("writing environment...")
	cfg.Env = append(cfg.Env,
		EnvPair{k: "KUBE_MASTER_IP", v: cfg.MasterIP},
		EnvPair{k: "KUBE_CLOUD_PROVIDER", v: cfg.CloudProvider})
	err = writeEnvironment(
		filepath.Join(rootfs, "etc", "container-environment"),
		cfg.Env)
	if err != nil {
		return err
	}

	err = writeConfig(filepath.Join(rootfs, "etc", "cloud-config"),
		cfg.CloudConfig)
	if err != nil {
		return err
	}

	root, err := libcontainer.New("/var/run/cube", libcontainer.Cgroupfs)
	if err != nil {
		return trace.Wrap(err)
	}

	containerID := uuid.New()

	config := &configs.Config{
		Rootfs:       rootfs,
		Capabilities: allCaps,
		Namespaces: configs.Namespaces([]configs.Namespace{
			{Type: configs.NEWNS},
			{Type: configs.NEWUTS},
			{Type: configs.NEWIPC},
			{Type: configs.NEWPID},
		}),
		Mounts: []*configs.Mount{
			{
				Source:      "/proc",
				Destination: "/proc",
				Device:      "proc",
				Flags:       defaultMountFlags,
			},
			// this is needed for flanneld that does modprobe
			{
				Device:      "bind",
				Source:      "/lib/modules",
				Destination: "/lib/modules",
				Flags:       defaultMountFlags | syscall.MS_BIND,
			},
			// don't mount real dev, otherwise systemd will mess up with the host
			// OS real badly
			{
				Source:      "tmpfs",
				Destination: "/dev",
				Device:      "tmpfs",
				Flags:       syscall.MS_NOSUID | syscall.MS_STRICTATIME,
				Data:        "mode=755",
			},
			{
				Source:      "sysfs",
				Destination: "/sys",
				Device:      "sysfs",
				Flags:       defaultMountFlags | syscall.MS_RDONLY,
			},
			{
				Source:      "devpts",
				Destination: "/dev/pts",
				Device:      "devpts",
				Flags:       syscall.MS_NOSUID | syscall.MS_NOEXEC,
				Data:        "newinstance,ptmxmode=0666,mode=0620,gid=5",
			},
		},
		Cgroups: &configs.Cgroup{
			Name:            containerID,
			Parent:          "system",
			AllowAllDevices: false,
			AllowedDevices:  configs.DefaultAllowedDevices,
		},

		Devices:  configs.DefaultAutoCreatedDevices,
		Hostname: containerID,
	}

	for _, m := range cfg.Mounts {
		src, err := checkPath(m.src, false)
		if err != nil {
			return trace.Wrap(err)
		}
		config.Mounts = append(config.Mounts, &configs.Mount{
			Device:      "bind",
			Source:      src,
			Destination: m.dst,
			Flags:       syscall.MS_BIND,
		})
	}

	container, err := root.Create(containerID, config)
	if err != nil {
		return trace.Wrap(err)
	}

	st, err := container.Status()
	if err != nil {
		return trace.Wrap(err)
	}
	log.Printf("container status: %v %v", st, err)

	process := &libcontainer.Process{
		Args:   []string{"/bin/systemd"},
		Env:    []string{"container=libcontainer", "TERM=xterm"},
		User:   "root",
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}

	// this will cause libcontainer to exec this binary again
	// with "init" command line argument.  (this is the default setting)
	// then our init() function comes into play
	if err := container.Start(process); err != nil {
		return trace.Wrap(err)
	}

	if err := startServer(serverSockPath(cfg.Rootfs), container); err != nil {
		return err
	}

	go monitorMasterUnits(container)

	// wait for the process to finish.
	status, err := process.Wait()
	if err != nil {
		return trace.Wrap(err)
	}

	log.Printf("process status: %v %v", status, err)

	container.Destroy()
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
				log.Printf("error getting status: %v", err)
			}
			units[u] = status
		}

		for u, s := range units {
			log.Printf("%v[%v]", strings.ToUpper(u), strings.ToUpper(s))
		}
		if allUnitsActive(units) {
			log.Printf("all units are up")
			return
		}
		time.Sleep(time.Second)
	}
}

func allUnitsActive(units map[string]string) bool {
	for _, s := range units {
		if s != "active" {
			return false
		}
	}
	return true
}

func getStatus(c libcontainer.Container, unit string) (string, error) {
	out, err := combinedOutput(
		c, ProcessConfig{
			User: "root",
			Args: []string{"/bin/systemctl", "is-active", unit}})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func writeEnvironment(path string, env EnvVars) error {
	f, err := os.Create(path)
	if err != nil {
		return trace.Wrap(err)
	}
	defer f.Close()
	for _, v := range env {
		if _, err := fmt.Fprintf(f, "%v=%v\n", v.k, v.v); err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
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

func checkPath(p string, executable bool) (string, error) {
	if p == "" {
		return "", trace.Errorf("path to root filesystem can not be empty")
	}
	cp, err := filepath.Abs(p)
	if err != nil {
		return "", trace.Wrap(err)
	}
	fi, err := os.Stat(cp)
	if err != nil {
		return "", trace.Wrap(err)
	}
	if executable && (fi.Mode()&0111 == 0) {
		return "", trace.Errorf("file %v is not executable", cp)
	}
	return cp, nil
}

const defaultMountFlags = syscall.MS_NOEXEC | syscall.MS_NOSUID | syscall.MS_NODEV

func parseKernel() (int, int, error) {
	uts := &syscall.Utsname{}

	if err := syscall.Uname(uts); err != nil {
		return 0, 0, trace.Wrap(err)
	}

	rel := bytes.Buffer{}
	for _, b := range uts.Release {
		if b == 0 {
			break
		}
		rel.WriteByte(byte(b))
	}

	var kernel, major int

	parsed, err := fmt.Sscanf(rel.String(), "%d.%d", &kernel, &major)
	if err != nil || parsed < 2 {
		return 0, 0, trace.Wrap(err, "can't parse kernel version")
	}
	return kernel, major, nil

}

func writeConfig(target, source string) error {
	bytes := []byte{}
	var err error
	if source != "" {
		bytes, err = ioutil.ReadFile(source)
		if err != nil {
			return err
		}
	}
	if err != nil {
		return trace.Wrap(ioutil.WriteFile(target, bytes, 0644))
	}
	return nil
}

// Return a nil error if the kernel supports aufs
// We cannot modprobe because inside dind modprobe fails
// to run
func supportsAufs() error {
	// We can try to modprobe aufs first before looking at
	// proc/filesystems for when aufs is supported
	err := exec.Command("modprobe", "aufs").Run()
	if err != nil {
		return trace.Wrap(err)
	}

	f, err := os.Open("/proc/filesystems")
	if err != nil {
		return trace.Wrap(err, "can't open /proc/filesystems")
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		if strings.Contains(s.Text(), "aufs") {
			return nil
		}
	}
	return trace.Errorf(
		"please install aufs driver support" +
			"On Ubuntu 'sudo apt-get install linux-image-extra-$(uname -r)'")
}

func startServer(path string, c libcontainer.Container) error {
	l, err := net.Listen("unix", path)
	if err != nil {
		return trace.Wrap(err)
	}
	srv := &http.Server{
		Handler: NewServer(c),
	}
	go func() {
		defer func() {
			if err := os.Remove(path); err != nil {
				log.Printf("failed to remove socket file: %v", err)
			}
		}()
		if err := srv.Serve(l); err != nil {
			log.Printf("server stopped with: %v", err)
		}
	}()
	return nil
}

func serverSockPath(p string) string {
	return filepath.Join(p, "run", "cube.socket")
}
