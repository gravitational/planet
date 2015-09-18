//+build linux
package box

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
)

var expectedSet = map[string]bool{
	"blkio":            true,
	"cpu,cpuacct":      true,
	"cpuset":           true,
	"devices":          true,
	"freezer":          true,
	"hugetlb":          true,
	"memory":           true,
	"net_cls,net_prio": true,
	"perf_event":       true,
}

const cgroupRootDir = "/sys/fs/cgroup"

func mountCgroups(root string) error {
	// read /proc/cgroups:
	foundCgroups, err := parseHostCgroups()
	if err != nil {
		return err
	}

	// read /proc/mounts
	mounts, err := parseMounts()
	if err != nil {
		return err
	}

	// find cgroups that have been mounted:
	mountedCgroups := make(map[string]bool)
	for group, _ := range foundCgroups {
		if mounts[path.Join(cgroupRootDir, group)] {
			mountedCgroups[group] = true
		}
	}
	log.Infof("Mounted cgroups: %v", mountedCgroups)

	// find potential cgroup conflicts
	cs := findConflicts(expectedSet, mountedCgroups)
	if len(cs) != 0 {
		return trace.Wrap(&CgroupConflictError{C: cs})
	}

	// mount cgroupfs (tmpfs root dir for cgroups) if it has not
	// been mounted already
	tmpfsRootDir := filepath.Join(root, cgroupRootDir)
	if !mounts[tmpfsRootDir] {
		if err := mountCgroupFS(tmpfsRootDir); err != nil {
			return err
		}
	} else {
		log.Infof("cgroup root (%v) is already mounted", cgroupRootDir)
	}

	var flags uintptr
	for c, _ := range expectedSet {
		if mountedCgroups[c] {
			log.Infof("%v is already mounted", c)
			continue
		}

		// skip unavaliable cgroups
		if !foundCgroups[c] {
			log.Infof("%v is not available, skip mounting", c)
			continue
		}

		// mount!
		cPath := filepath.Join(root, cgroupRootDir, c)
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
	cgroups := make(map[int][]string)

	subs := make(map[string]bool)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") { // skip comments
			continue
		}
		fmt.Sscanf(line, "%s %d %d %d", &controller, &hierarchy, &num, &enabled)
		if enabled == 1 {
			if _, ok := cgroups[hierarchy]; !ok {
				cgroups[hierarchy] = []string{controller}
			} else {
				cgroups[hierarchy] = append(cgroups[hierarchy], controller)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, trace.Wrap(err)
	}
	for _, gs := range cgroups {
		subs[strings.Join(gs, ",")] = true
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

func findConflicts(a, b map[string]bool) conflicts {
	c := make(map[string][]string)
	for g1, _ := range a {
		for g2, _ := range b {
			if intersect(g1, g2) {
				c[g1] = append(c[g1], g2)
			}
		}
	}
	return c
}

func intersect(a, b string) bool {
	if a == b {
		return false
	}
	avals := makeSet(strings.Split(a, ","))
	bvals := makeSet(strings.Split(b, ","))
	for k, _ := range avals {
		if bvals[k] {
			return true
		}
	}
	return false
}

func makeSet(a []string) map[string]bool {
	out := make(map[string]bool, len(a))
	for _, v := range a {
		out[v] = true
	}
	return out
}

type CgroupConflictError struct {
	C conflicts
}

func (e *CgroupConflictError) Error() string {
	return e.C.String()
}

type conflicts map[string][]string

func (c conflicts) String() string {
	if len(c) == 0 {
		return "<no-conflicts>"
	}
	b := &bytes.Buffer{}
	for a, c := range c {
		fmt.Fprintf(b, "cgroup '%v' conflicts with %v, ", a, c)
	}
	return b.String()
}
