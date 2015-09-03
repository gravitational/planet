package main

import (
	"fmt"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/orbit/lib/utils"
	"github.com/gravitational/planet/lib/box"
)

type Config struct {
	Roles         roles
	Rootfs        string
	MasterIP      string
	CloudProvider string
	CloudConfig   string
	Env           box.EnvVars
	Mounts        box.Mounts
	IgnoreChecks  bool
}

func (cfg *Config) hasRole(r string) bool {
	for _, rs := range cfg.Roles {
		if rs == r {
			return true
		}
	}
	return false
}

type roles []string

func (rs *roles) Set(role string) error {
	for _, r := range utils.SplitComma(role) {
		*rs = append(*rs, r)
	}
	return nil
}

func (rs *roles) String() string {
	return fmt.Sprintf("%v", []string(*rs))
}
