package main

import (
	"fmt"
	"log"

	"github.com/gravitational/go-udev"
	"github.com/gravitational/trace"
)

func main() {
	if err := run(); err != nil {
		log.Fatalln(err)
	}
}

func run() error {
	// Enumerate all known block devices of type disk/partition
	udev := udev.Udev{}
	enum := udev.NewEnumerate()
	devices, err := enum.Devices()
	if err != nil {
		return trace.Wrap(err, "failed to enumerate available devices")
	}
	for _, device := range devices {
		fmt.Printf("Type: %v, path: %v.\n", device.Devtype(), device.Devpath())
	}
	return nil
}
