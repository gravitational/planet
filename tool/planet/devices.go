package main

import (
	"os"
	"path/filepath"
	"syscall"

	"github.com/gravitational/trace"

	"github.com/opencontainers/runc/libcontainer/configs"
)

// createDevice creates a node for the specified device in the container
func createDevice(device *configs.Device) error {
	oldMask := syscall.Umask(0000)
	if err := createDeviceNode(device); err != nil {
		syscall.Umask(oldMask)
		return trace.Wrap(err)
	}
	syscall.Umask(oldMask)
	return nil
}

// removeDevice removes the device specified with node path
func removeDevice(node string) error {
	return os.Remove(node)
}

// createDeviceNode creates the device node inside the container.
func createDeviceNode(node *configs.Device) error {
	dest := node.Path
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return trace.Wrap(err)
	}

	if err := mknodDevice(dest, node); err != nil && !os.IsExist(err) {
		return trace.Wrap(err)
	}
	return nil
}

// mknodDevice creates the device node inside the container using `mknod`
func mknodDevice(dest string, node *configs.Device) error {
	fileMode := node.FileMode
	switch node.Type {
	case 'c':
		fileMode |= syscall.S_IFCHR
	case 'b':
		fileMode |= syscall.S_IFBLK
	default:
		return trace.Errorf("%c is not a valid device type for device %s", node.Type, node.Path)
	}
	if err := syscall.Mknod(dest, uint32(fileMode), node.Mkdev()); err != nil {
		return trace.Wrap(err)
	}
	return syscall.Chown(dest, int(node.Uid), int(node.Gid))
}
