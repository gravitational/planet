//+build linux

package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func MayBeMountCgroups(root string) error {
	files, err := ioutil.ReadDir(filepath.Join(root, "sys", "fs", "cgroup"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	cgroups, err := parseCgroups()
	if err != nil {
		return fmt.Errorf("error parsing /proc/cgroups: %v", err)
	}

	// try to find at least one mounted cgroup
	for _, c := range cgroups {
		for _, f := range files {
			if strings.Contains(f.Name(), c) {
				log.Printf("found mounted cgroup: %v, returning", c)
				return nil
			}
		}
	}

	// Mount /sys/fs/cgroup
	cgroupTmpfs := filepath.Join(root, "/sys/fs/cgroup")
	if err := os.MkdirAll(cgroupTmpfs, 0700); err != nil {
		return err
	}

	var flags uintptr
	flags = syscall.MS_NOSUID |
		syscall.MS_NOEXEC |
		syscall.MS_NODEV |
		syscall.MS_STRICTATIME
	if err := syscall.Mount("tmpfs", cgroupTmpfs, "tmpfs", flags, "mode=755"); err != nil {
		return fmt.Errorf("error mounting %q: %v", cgroupTmpfs, err)
	}

	for _, c := range cgroups {
		cPath := filepath.Join(root, "/sys/fs/cgroup", c)
		if err := os.MkdirAll(cPath, 0700); err != nil {
			return err
		}

		flags = syscall.MS_NOSUID |
			syscall.MS_NOEXEC |
			syscall.MS_NODEV
		log.Printf("mount: %v %v %v %v %v", "cgroup", cPath, "cgroup", flags, c)
		if err := syscall.Mount("cgroup", cPath, "cgroup", flags, c); err != nil {
			return fmt.Errorf("error mounting %q: %v", cPath, err)
		}
	}
	return nil
}

func parseCgroups() (subs []string, err error) {
	// open /proc/cgroups
	f, err := os.Open("/proc/cgroups")
	if err != nil {
		return subs, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "#") { // skip comments
			fields := strings.Fields(line)
			if len(fields) > 0 {
				subs = append(subs, fields[0])
			}
		}
	}
	return subs, nil
}
