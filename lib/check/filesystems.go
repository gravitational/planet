package check

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/gravitational/trace"
)

var (
	mountedFS map[string]string = make(map[string]string)
)

// IsBtrfsVolume determines if dirPath is located on BTRFS filesystem
func IsBtrfsVolume(dirPath string) (isBtrfs bool, err error) {
	fs, err := fsForDir(dirPath)
	if err != nil {
		return false, trace.Wrap(err)
	}
	return (fs == "btrfs"), nil
}

// fsForDir returns the filesystem name which hosts a given directory
func fsForDir(dirPath string) (fs string, err error) {
	dirPath, err = filepath.Abs(filepath.Clean(dirPath))
	if err != nil {
		return fs, trace.Errorf("Invalid path: %v. Error: %v", dirPath, err)
	}
	// read /proc/mounts:
	mounts, err := ParseMountsFile()
	if err != nil {
		return fs, trace.Wrap(err)
	}
	var match string
	for mountPoint, fsType := range mounts {
		if strings.HasPrefix(dirPath, mountPoint) {
			if len(match) < len(mountPoint) {
				match = mountPoint
				fs = fsType
			}
		}
	}
	return fs, nil
}

// ParseMountsFile reads and parses /proc/mounts
// Returns a map "mount point" -> "fs type"
func ParseMountsFile() (map[string]string, error) {
	mounts := make(map[string]string)
	// avoid re-reading the file if mounts have previously been
	// parsed:
	if len(mountedFS) == 0 {
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
			mountFS := fields[2]
			mountName := fields[1]
			mountedFS[mountName] = mountFS
		}
		if err := sc.Err(); err != nil {
			return nil, trace.Wrap(err)
		}
	}
	// return a copy:
	for k, v := range mountedFS {
		mounts[k] = v
	}
	return mounts, nil
}
