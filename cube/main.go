package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"code.google.com/p/go-uuid/uuid"
	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/configs"
)

func init() {
	fmt.Printf("inside init: %v\n", os.Args)
	if len(os.Args) > 1 && os.Args[1] == "init" {
		runtime.GOMAXPROCS(1)
		runtime.LockOSThread()
		factory, _ := libcontainer.New("")
		if err := factory.StartInitialization(); err != nil {
			log.Fatal(err)
		}
		panic("--this line should have never been executed, congratulations--")
	}
}

func writeEnvironment(path string, env envVars) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, v := range env {
		if _, err := fmt.Fprintf(f, "%v=%v\n", v.k, v.v); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	fmt.Printf("called: %v\n", os.Args)

	if os.Geteuid() != 0 {
		log.Fatal("cube should be run as root")
	}
	k, m, err := parseKernel()
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("kernel: %v.%v\n", k, m)
	if k*100+m != 313 {
		log.Fatal(fmt.Errorf(
			"current supported kernel version is 3.13. Upgrade kernel before moving on."))
	}

	if err := supportsAufs(); err != nil {
		log.Fatal(err)
	}

	var cname, masterIP, cloud, cloudConfig string
	vals := envVars{}

	flag.StringVar(&cname, "name", "", "container name")
	flag.StringVar(&masterIP, "master-ip", "127.0.0.1", "master ip")
	flag.StringVar(&cloud, "cloud-provider", "", "cloud provider")
	flag.StringVar(&cloudConfig, "cloud-config", "", "cloud config")
	flag.Var(&vals, "env", "set environment variable")
	flag.Parse()

	if cname == "" {
		cname = uuid.New()
	}

	args := flag.Args()
	if len(args) < 1 {
		log.Fatalf("cube /path/to/rootfs")
	}

	params := []string{"/bin/systemd"}
	if len(args) > 1 {
		params = args[1:]
	}

	rootfs, err := checkPath(args[0], false)
	if err != nil {
		log.Fatalf("cube error: %v", err)
	}

	if err := MayBeMountCgroups("/"); err != nil {
		log.Fatal(err)
	}

	log.Printf("starting container process in '%v'", rootfs)

	log.Printf("writing environment...")
	vals = append(vals,
		envPair{k: "KUBE_MASTER_IP", v: masterIP},
		envPair{k: "KUBE_CLOUD_PROVIDER", v: cloud})
	err = writeEnvironment(
		filepath.Join(rootfs, "etc", "container-environment"),
		vals)
	if err != nil {
		log.Fatal(err)
	}

	err = writeConfig(filepath.Join(rootfs, "etc", "cloud-config"),
		cloudConfig)
	if err != nil {
		log.Fatal(err)
	}

	root, err := libcontainer.New("/var/run/cube", libcontainer.Cgroupfs)
	if err != nil {
		log.Fatalf("cube: %v", err)
	}

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
			Name:            cname,
			Parent:          "system",
			AllowAllDevices: false,
			AllowedDevices:  configs.DefaultAllowedDevices,
		},

		Devices:  configs.DefaultAutoCreatedDevices,
		Hostname: cname,
	}

	container, err := root.Create(uuid.New(), config)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	st, err := container.Status()
	log.Printf("container status: %v %v", st, err)

	process := &libcontainer.Process{
		Args:   params,
		Env:    []string{"container=libcontainer"},
		User:   "root",
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}

	// this will cause libcontainer to exec this binary again
	// with "init" command line argument.  (this is the default setting)
	// then our init() function comes into play
	if err := container.Start(process); err != nil {
		log.Fatal(err)
	}

	// wait for the process to finish.
	status, err := process.Wait()
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("process status: %v %v", status, err)

	// destroy the container.
	container.Destroy()
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
	cp, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	fi, err := os.Stat(cp)
	if err != nil {
		return "", err
	}
	if executable && (fi.Mode()&0111 == 0) {
		return "", fmt.Errorf("file %v is not executable", cp)
	}
	return cp, nil
}

const defaultMountFlags = syscall.MS_NOEXEC | syscall.MS_NOSUID | syscall.MS_NODEV

func parseKernel() (int, int, error) {
	uts := &syscall.Utsname{}

	if err := syscall.Uname(uts); err != nil {
		return 0, 0, err
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
		return 0, 0, fmt.Errorf("Can't parse kernel version")
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
	} else {
	}
	return ioutil.WriteFile(target, bytes, 0644)
}

type envPair struct {
	k string
	v string
}

type envVars []envPair

func (vars *envVars) Set(v string) error {
	vals := strings.Split(v, "=")
	if len(vals) != 2 {
		return fmt.Errorf(
			"set environment variable separated by '=', e.g. KEY=VAL")
	}
	*vars = append(*vars, envPair{k: vals[0], v: vals[1]})
	return nil
}

func (vars *envVars) String() string {
	if len(*vars) == 0 {
		return ""
	}
	b := &bytes.Buffer{}
	for i, v := range *vars {
		fmt.Fprintf(b, "%v=%v", v.k, v.v)
		if i != len(*vars)-1 {
			fmt.Fprintf(b, " ")
		}
	}
	return b.String()
}

// Return a nil error if the kernel supports aufs
// We cannot modprobe because inside dind modprobe fails
// to run
func supportsAufs() error {
	// We can try to modprobe aufs first before looking at
	// proc/filesystems for when aufs is supported
	exec.Command("modprobe", "aufs").Run()

	f, err := os.Open("/proc/filesystems")
	if err != nil {
		return err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		if strings.Contains(s.Text(), "aufs") {
			return nil
		}
	}
	return fmt.Errorf("please install aufs driver support. If it's Ubuntu 'sudo apt-get install linux-image-extra-$(uname -r)'")
}
