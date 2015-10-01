package main

import (
	"fmt"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/orbit/lib/utils"
	"github.com/gravitational/planet/lib/box"
)

type Config struct {
	Roles              list
	InsecureRegistries list
	Rootfs             string
	MasterIP           string
	CloudProvider      string
	ClusterID          string
	Env                box.EnvVars
	Mounts             box.Mounts
	Files              []box.File
	IgnoreChecks       bool
}

func (cfg *Config) hasRole(r string) bool {
	for _, rs := range cfg.Roles {
		if rs == r {
			return true
		}
	}
	return false
}

type list []string

func (l *list) Set(val string) error {
	for _, r := range utils.SplitComma(val) {
		*l = append(*l, r)
	}
	return nil
}

func (l *list) String() string {
	return fmt.Sprintf("%v", []string(*l))
}
