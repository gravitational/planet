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

package main

import (
	"os"
	"path/filepath"
	"syscall"

	"github.com/gravitational/trace"

	"github.com/opencontainers/runc/libcontainer/devices"
)

// createDevice creates a node for the specified device in the container
func createDevice(device *devices.Device) error {
	oldMask := syscall.Umask(0000)
	if err := createDeviceNode(device); err != nil {
		syscall.Umask(oldMask)
		return trace.Wrap(err)
	}
	syscall.Umask(oldMask)
	return nil
}

// removeDevice removes the device specified with node path
func removeDevice(node string) (err error) {
	if err = os.Remove(node); err != nil && os.IsNotExist(err) {
		// Ignore `file not found` errors
		return nil
	}
	return trace.Wrap(err)
}

// createDeviceNode creates the device node inside the container.
func createDeviceNode(node *devices.Device) error {
	dest := node.Path
	if err := os.MkdirAll(filepath.Dir(dest), 0777); err != nil {
		return trace.Wrap(err)
	}

	if err := mknodDevice(dest, node); err != nil && !os.IsExist(err) {
		return trace.Wrap(err)
	}
	return nil
}

// mknodDevice creates the device node inside the container using `mknod`
func mknodDevice(dest string, node *devices.Device) error {
	fileMode := node.FileMode
	switch node.Type {
	case 'c':
		fileMode |= syscall.S_IFCHR
	case 'b':
		fileMode |= syscall.S_IFBLK
	default:
		return trace.Errorf("%c is not a valid device type for device %s", node.Type, node.Path)
	}

	dev, err := node.Mkdev()
	if err != nil {
		return trace.Wrap(err)
	}

	if err := syscall.Mknod(dest, uint32(fileMode), int(dev)); err != nil && !os.IsExist(err) {
		return trace.Wrap(err, "failed to create node for %v (mode=%v)", dest, fileMode)
	}
	return syscall.Chown(dest, int(node.Uid), int(node.Gid))
}
