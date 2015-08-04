package main

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/gravitational/cube/Godeps/_workspace/src/github.com/gravitational/trace"
)

type CubeConfig struct {
	Rootfs        string
	MasterIP      string
	CloudProvider string
	CloudConfig   string
	Env           EnvVars
	Mounts        Mounts
	Force         bool
}

type EnvPair struct {
	k string
	v string
}

type EnvVars []EnvPair

func (vars *EnvVars) Set(v string) error {
	vals := strings.Split(v, "=")
	if len(vals) != 2 {
		return trace.Errorf(
			"set environment variable separated by '=', e.g. KEY=VAL")
	}
	*vars = append(*vars, EnvPair{k: vals[0], v: vals[1]})
	return nil
}

func (vars *EnvVars) String() string {
	if len(*vars) == 0 {
		return ""
	}
	b := &bytes.Buffer{}
	for i, v := range *vars {
		fmt.Fprintf(b, "%v=%v", v.k, v.v)
		if i != len(*vars)-1 {
			fmt.Fprintf(b, " ")
		}
	}
	return b.String()
}

type Mount struct {
	src string
	dst string
}

type Mounts []Mount

func (m *Mounts) Set(v string) error {
	vals := strings.Split(v, ":")
	if len(vals) != 2 {
		return trace.Errorf(
			"set mounts separated by : e.g. src:dst")
	}
	*m = append(*m, Mount{src: vals[0], dst: vals[1]})
	return nil
}

func (m *Mounts) String() string {
	if len(*m) == 0 {
		return ""
	}
	b := &bytes.Buffer{}
	for i, v := range *m {
		fmt.Fprintf(b, "%v:%v", v.src, v.dst)
		if i != len(*m)-1 {
			fmt.Fprintf(b, " ")
		}
	}
	return b.String()
}
