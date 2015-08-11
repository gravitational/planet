package main

import (
	"fmt"

	"github.com/gravitational/cube/Godeps/_workspace/src/github.com/gravitational/orbit/box"
)

type CubeConfig struct {
	Roles         roles
	Rootfs        string
	MasterIP      string
	CloudProvider string
	CloudConfig   string
	Env           box.EnvVars
	Mounts        box.Mounts
	Force         bool
}

func (cfg *CubeConfig) hasRole(r string) bool {
	for _, rs := range cfg.Roles {
		if rs == r {
			return true
		}
	}
	return false
}

type roles []string

func (r *roles) Set(role string) error {
	*r = append(*r, role)
	return nil
}

func (r *roles) String() string {
	return fmt.Sprintf("%v", []string(*r))
}
