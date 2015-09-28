//+build linux
package box

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
)

// planet won't start without these groups enabled in the kernel
var requiredSet = map[string]bool{
	"blkio":   true,
	"cpu":     true,
	"cpuacct": true,
	"cpuset":  true,
	"devices": true,
	"memory":  true,
}

// by default try to mount these cgroups if they aren't mounted
var expectedSet = map[string]string{
	"blkio":            "0",
	"cpu,cpuacct":      "0",
	"cpuset":           "0",
	"devices":          "0",
	"freezer":          "0",
	"memory":           "0",
	"net_cls,net_prio": "0",
	"perf_event":       "0",
	"net_prio":         "net_cls,net_prio",
	"net_cls":          "net_cls,net_prio",
	"cpu":              "cpu,cpuacct", // make symlink "cpu -> cpu,cpuacct"
	"cpuacct":          "cpu,cpuacct",
}

const cgroupRootDir = "/sys/fs/cgroup"

func MountCgroups(root string) error {
	// read /proc/cgroups:
	foundCgroups, err := parseHostCgroups()
	if err != nil {
		return err
	}
	log.Infof("\n------\nFound cgroups in /proc/cgroup:\n %v", foundCgroups)

	// make sure they're all enabled in kernel:
	for group, _ := range requiredSet {
		if !foundCgroups[group] {
			err := trace.Errorf("cgroup '%v' is requred, but not found in /proc/cgroups", group)
			if group == "memory" {
				log.Errorf("To enable memory cgroup on Debian/Ubuntu see https://docs.docker.com/installation/ubuntulinux/")
			}
			return err
		}
	}

	// read /proc/mounts
	mounts, err := parseMounts()
	if err != nil {
		return err
	}

	// mount cgroupfs root (/sys/fs/cgroup itself) if it has not been mounted already
	tmpfsRootDir := filepath.Join(root, cgroupRootDir)
	if !mounts[tmpfsRootDir] {
		if err := mountCgroupFS(tmpfsRootDir); err != nil {
			return err
		}
	} else {
		log.Infof("cgroup root (%v) is already mounted", cgroupRootDir)
	}

GroupCycle:
	for group, linksTo := range expectedSet {
		// but first, check if this group is present in /proc/cgroups:
		for _, group := range strings.Split(group, ",") {
			if !foundCgroups[group] {
				log.Infof("cgroup \"%v\" is not available, skipping mounting", group)
				continue GroupCycle
			}
		}
		// linksTo may be pointing to a symlink target or "0" if a new mount needs
		// to be created
		if linksTo == "0" {
			cPath := filepath.Join(root, cgroupRootDir, group)
			if mounts[cPath] {
				log.Infof("cgroup \"%v\" is already mounted", group)
				continue GroupCycle
			}

			if err := os.MkdirAll(cPath, 0700); err != nil {
				return trace.Wrap(err)
			}
			var flags uintptr = syscall.MS_NOSUID |
				syscall.MS_NOEXEC |
				syscall.MS_NODEV
			log.Infof("mount: %v %v %v %v %v", "cgroup", cPath, "cgroup", flags, group)
			if err := syscall.Mount("cgroup", cPath, "cgroup", flags, group); err != nil {
				return trace.Errorf("error mounting %q: %v", cPath, err)
			}
			// instead of mounting, create a symlink:
		} else {
			cPath := filepath.Join(root, cgroupRootDir, group)
			if err := os.Symlink(linksTo, cPath); err != nil {
				if os.IsExist(err) {
					log.Infof("%v is already symlinked", group)
				} else {
					return trace.Errorf("Tried to symlink %v -> %v and failed: %v", cPath, linksTo, err)
				}
			}
		}
	}

	return nil
}

// mountCgroupFS mounts /sys/fs/cgroup
func mountCgroupFS(cgroupTmpfs string) error {
	log.Infof("creating tmpfs for cgrups in %v", cgroupTmpfs)

	if err := os.MkdirAll(cgroupTmpfs, 0700); err != nil {
		return trace.Wrap(err)
	}

	var flags uintptr
	flags = syscall.MS_NOSUID |
		syscall.MS_NOEXEC |
		syscall.MS_NODEV |
		syscall.MS_STRICTATIME
	if err := syscall.Mount("tmpfs", cgroupTmpfs, "tmpfs", flags, "mode=755"); err != nil {
		if err == syscall.EBUSY { // already mounted
			log.Infof("%v is already mounted", cgroupTmpfs)
			return nil
		}
		return trace.Wrap(err, fmt.Sprintf("error mounting %v", cgroupTmpfs))
	}
	return nil
}

// parseHostCgroups opens and parses the cgroup file (/proc/cgroups)
func parseHostCgroups() (map[string]bool, error) {
	f, err := os.Open("/proc/cgroups")
	if err != nil {
		return nil, trace.Wrap(err)
	}
	defer f.Close()

	var (
		controller              string
		hierarchy, num, enabled int
	)
	subs := make(map[string]bool)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") { // skip comments
			continue
		}
		fmt.Sscanf(line, "%s %d %d %d", &controller, &hierarchy, &num, &enabled)
		// only accept enabled cgroups
		if enabled == 1 {
			subs[controller] = true
		}
	}
	if err := sc.Err(); err != nil {
		return nil, trace.Wrap(err)
	}
	return subs, nil
}

// parseMounts reads and parses /proc/mounts file
func parseMounts() (map[string]bool, error) {
	mounts := make(map[string]bool)
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return nil, trace.Wrap(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)

	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") { // skip comments
			continue
		}
		fields := strings.Fields(line)
		mounts[fields[1]] = true
	}
	if err := sc.Err(); err != nil {
		return nil, trace.Wrap(err)
	}
	return mounts, nil
}
