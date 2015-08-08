package main

import (
	"github.com/gravitational/cube/Godeps/_workspace/src/github.com/gravitational/orbit/box"
)

type CubeConfig struct {
	Role          string
	Rootfs        string
	MasterIP      string
	CloudProvider string
	CloudConfig   string
	Env           box.EnvVars
	Mounts        box.Mounts
	Force         bool
}
