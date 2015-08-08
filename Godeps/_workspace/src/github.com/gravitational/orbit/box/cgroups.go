//+build linux

package box

import (
	"bufio"

	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/gravitational/cube/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/cube/Godeps/_workspace/src/github.com/gravitational/trace"
)

func mayBeMountCgroups(root string) error {
	files, err := ioutil.ReadDir(filepath.Join(root, "sys", "fs", "cgroup"))
	if err != nil && !os.IsNotExist(err) {
		return trace.Wrap(err)
	}

	cgroups, err := parseCgroups()
	if err != nil {
		return err
	}

	// try to find at least one mounted cgroup
	for c, _ := range cgroups {
		for _, f := range files {
			if strings.Contains(f.Name(), c) {
				log.Infof("found mounted cgroup: %v, returning", c)
				return nil
			}
		}
	}

	// Mount /sys/fs/cgroup
	cgroupTmpfs := filepath.Join(root, "/sys/fs/cgroup")
	if err := os.MkdirAll(cgroupTmpfs, 0700); err != nil {
		return trace.Wrap(err)
	}

	// group cgroups in systemd style
	groupCgroups(cgroups)

	var flags uintptr
	flags = syscall.MS_NOSUID |
		syscall.MS_NOEXEC |
		syscall.MS_NODEV |
		syscall.MS_STRICTATIME
	if err := syscall.Mount("tmpfs", cgroupTmpfs, "tmpfs", flags, "mode=755"); err != nil {
		return trace.Errorf("error mounting %q: %v", cgroupTmpfs, err)
	}

	for c, _ := range cgroups {
		cPath := filepath.Join(root, "/sys/fs/cgroup", c)
		if err := os.MkdirAll(cPath, 0700); err != nil {
			return trace.Wrap(err)
		}

		flags = syscall.MS_NOSUID |
			syscall.MS_NOEXEC |
			syscall.MS_NODEV
		log.Infof("mount: %v %v %v %v %v", "cgroup", cPath, "cgroup", flags, c)
		if err := syscall.Mount("cgroup", cPath, "cgroup", flags, c); err != nil {
			return trace.Errorf("error mounting %q: %v", cPath, err)
		}
	}
	return nil
}

func parseCgroups() (subs map[string]bool, err error) {
	subs = make(map[string]bool)
	// open /proc/cgroups
	f, err := os.Open("/proc/cgroups")
	if err != nil {
		return subs, trace.Wrap(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "#") { // skip comments
			fields := strings.Fields(line)
			if len(fields) > 0 {
				subs[fields[0]] = true
			}
		}
	}
	return subs, nil
}

// group Cgroups in systemd style
func groupCgroups(in map[string]bool) {
	if in["cpu"] && in["cpuacct"] {
		in["cpu,cpuacct"] = true
		delete(in, "cpu")
		delete(in, "cpuacct")
	}
}
