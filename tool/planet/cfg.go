package main

import (
	"fmt"
	"net"
	"os/user"
	"strconv"
	"strings"

	"github.com/gravitational/planet/lib/box"

	kv "github.com/gravitational/configure"
	"github.com/gravitational/configure/cstrings"
)

type Config struct {
	Roles                   list
	InsecureRegistries      list
	Rootfs                  string
	SocketPath              string
	PublicIP                string
	MasterIP                string
	CloudProvider           string
	ClusterID               string
	Env                     box.EnvVars
	Mounts                  box.Mounts
	Files                   []box.File
	IgnoreChecks            bool
	SecretsDir              string
	StateDir                string
	DockerBackend           string
	DockerOptions           string
	ServiceSubnet           kv.CIDR
	PODSubnet               kv.CIDR
	InitialCluster          kv.KeyVal
	ServiceUser             *user.User
	ServiceUID              string
	ServiceGID              string
	EtcdProxy               string
	EtcdMemberName          string
	EtcdInitialCluster      string
	EtcdInitialClusterState string
	ElectionEnabled         bool
	NodeName                string
}

func (cfg *Config) SkyDNSResolverIP() string {
	return cfg.ServiceSubnet.RelativeIP(3).String()
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
	for _, r := range cstrings.SplitComma(val) {
		*l = append(*l, r)
	}
	return nil
}

func (l *list) String() string {
	return fmt.Sprintf("%v", []string(*l))
}

// hostPort is a command line flag that understands input
// as a host:port pair.
type hostPort struct {
	host string
	port int64
}

func (r *hostPort) Set(input string) error {
	var err error
	var port string

	r.host, port, err = net.SplitHostPort(input)
	if err != nil {
		return err
	}

	r.port, err = strconv.ParseInt(port, 0, 0)
	return err
}

func (r hostPort) String() string {
	return net.JoinHostPort(r.host, fmt.Sprintf("%v", r.port))
}

// toKeyValueList combines key/value pairs from kv into a comma-separated list.
func toKeyValueList(kv kv.KeyVal) string {
	var result []string
	for key, value := range kv {
		result = append(result, fmt.Sprintf("%v:%v", key, value))
	}
	return strings.Join(result, ",")
}

// boolFlag defines a boolean command line flag.
// The behavioral difference to the kingpin's built-in Bool() modifier
// is that it supports the long form:
// 	--flag=true|false
// as opposed to built-in's only short form:
//	--flag	(true, if specified, false - otherwise)
// The long form is required when populating the flag from the environment.
type boolFlag bool

func (r *boolFlag) Set(input string) error {
	if input == "" {
		input = "true"
	}
	value, err := strconv.ParseBool(input)
	*r = boolFlag(value)
	return err
}

func (r boolFlag) String() string {
	return strconv.FormatBool(bool(r))
}
