/*
Copyright 2018 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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

	"github.com/gravitational/trace"
)

// Return a nil error if the kernel supports a given filesystem (like "aufs" or
// "overlay")
func CheckFS(fs string) (bool, error) {
	err := exec.Command("modprobe", fs).Run()
	if err != nil && !isErrNotFound(err) {
		return false, nil
	}
	return findFS(fs)
}

// findFS opens /proc/filesystems and looks for a given filesystem name
func findFS(fs string) (bool, error) {
	f, err := os.Open("/proc/filesystems")
	if err != nil {
		return false, trace.Wrap(err, "can't open /proc/filesystems")
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		if strings.Contains(s.Text(), fs) {
			return true, nil
		}
	}
	return false, nil
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

func isErrNotFound(err error) bool {
	switch pe := err.(type) {
	case *exec.Error:
		return pe.Err == exec.ErrNotFound
	default:
		return os.IsNotExist(err)
	}
}
