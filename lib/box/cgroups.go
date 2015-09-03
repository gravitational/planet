//+build linux
package box

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
)

func mountCgroups(root string) error {
	cgroups, err := mountedCgroups(root)
	if err != nil {
		return err
	}

	// find potential cgroup conflicts
	cs := findConflicts(expectedSet, cgroups)
	if len(cs) != 0 {
		return trace.Wrap(&CgroupConflictError{C: cs})
	}

	// mount cgroupfs if it has not been mounted already
	if err := mountCgroupFS(root); err != nil {
		return err
	}

	var flags uintptr
	for c, _ := range expectedSet {
		if cgroups[c] {
			log.Infof("%v is already mounted", c)
			continue
		}
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

// mountCgroupFS mounts /sys/fs/cgroup
func mountCgroupFS(root string) error {
	cgroupTmpfs := filepath.Join(root, "/sys/fs/cgroup")
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

func mountedCgroups(root string) (map[string]bool, error) {
	cs, err := parseCgroups()
	if err != nil {
		return nil, err
	}
	ms, err := parseMounts()
	if err != nil {
		return nil, err
	}
	for g, _ := range cs {
		if !ms[g] {
			delete(cs, g)
		}
	}
	return cs, nil
}

func parseCgroups() (map[string]bool, error) {
	subs := make(map[string]bool)
	f, err := os.Open("/proc/cgroups")
	if err != nil {
		return nil, trace.Wrap(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)

	cgroups := make(map[int][]string)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") { // skip comments
			continue
		}
		var controller string
		var hierarchy int
		var num int
		var enabled int
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

	log.Infof("parsed active hierarchies: %v", cgroups)
	return subs, nil
}

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

func findConflicts(a, b groupset) conflicts {
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

type groupset map[string]bool

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
