// package check provides various checks for the operating system
// that are necessary software to work, e.g. version of the kernel
// whether aufs is supported, etc
package check

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
)

// Return a nil error if the kernel supports aufs
// We cannot modprobe because inside dind modprobe fails
// to run
func SupportsAufs() (bool, error) {
	// We can try to modprobe aufs first before looking at
	// proc/filesystems for when aufs is supported
	err := exec.Command("modprobe", "aufs").Run()
	if err != nil {
		return false, trace.Wrap(err)
	}

	f, err := os.Open("/proc/filesystems")
	if err != nil {
		return false, trace.Wrap(err, "can't open /proc/filesystems")
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		if strings.Contains(s.Text(), "aufs") {
			return true, nil
		}
	}
	return false, trace.Errorf(
		"please install aufs driver support" +
			"On Ubuntu 'sudo apt-get install linux-image-extra-$(uname -r)'")
}

func KernelVersion() (int, error) {
	a, b, err := parseKernel()
	if err != nil {
		return -1, err
	}
	return a*100 + b, nil
}

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
