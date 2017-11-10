package main

import (
	"github.com/gravitational/planet/lib/network"

	"github.com/gravitational/trace"
)

func enablePromiscMode(ifaceName, podCidr string) error {
	return trace.Wrap(network.SetPromiscuousMode(ifaceName, podCidr))
}

func disablePromiscMode(ifaceName string) error {
	return trace.Wrap(network.UnsetPromiscuousMode(ifaceName))
}
