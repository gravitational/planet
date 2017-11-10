package main

import (
	"github.com/gravitational/planet/lib/network"

	"github.com/gravitational/trace"
)

func linkPromiscMode(ifaceName, podCidr string) error {
	return trace.Wrap(network.SetPromiscuousMode(ifaceName, podCidr))
}
