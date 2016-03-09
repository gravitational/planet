package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/gravitational/trace"

	log "github.com/Sirupsen/logrus"
	"github.com/opencontainers/runc/libcontainer/configs"
	"gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	if err := run(); err != nil {
		log.Fatalln(err)
	}
}

func run() error {
	var (
		app   = kingpin.New("device", "Manage devices in container")
		debug = app.Flag("debug", "Verbose mode").Bool()

		cdeviceAdd     = app.Command("add", "Add new device to container")
		cdeviceAddData = cdeviceAdd.Flag("data", "Device definition as seen on host").Required().String()

		cdeviceRemove     = app.Command("remove", "Remove device from container")
		cdeviceRemoveNode = cdeviceRemove.Flag("node", "Device node to remove").Required().String()
	)

	log.SetOutput(os.Stderr)
	if *debug {
		log.SetLevel(log.WarnLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	cmd, err := app.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse command line: %v.\nUse deviced --help for help.\n", err)
		return trace.Wrap(err)
	}

	switch cmd {
	case cdeviceAdd.FullCommand():
		var device configs.Device
		if err = json.Unmarshal([]byte(*cdeviceAddData), &device); err != nil {
			break
		}
		err = createDevice(&device)
	case cdeviceRemove.FullCommand():
		err = removeDevice(*cdeviceRemoveNode)
	}
	// FIXME: vendor updated trace
	// return trace.Wrap(err)
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

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

func removeDevice(node string) error {
	return os.Remove(node)
}

// createDeviceNode creates the device node inside the container.
func createDeviceNode(node *configs.Device) error {
	dest := node.Path
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return trace.Wrap(err)
	}

	if err := mknodDevice(dest, node); err != nil {
		if os.IsExist(err) {
			return nil
		} else if os.IsPermission(err) {
			// FIXME
			return bindMountDeviceNode(dest, node)
		}
		return trace.Wrap(err)
	}
	return nil
}

// bindMountDeviceNode bind-mounts the specified device in dest
func bindMountDeviceNode(dest string, node *configs.Device) error {
	f, err := os.Create(dest)
	if err != nil && !os.IsExist(err) {
		return trace.Wrap(err)
	}
	if f != nil {
		f.Close()
	}
	return syscall.Mount(node.Path, dest, "bind", syscall.MS_BIND, "")
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
