package main

import (
	"fmt"
	"net"
	"os/user"
	"strconv"
	"strings"

	"github.com/gravitational/planet/lib/box"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/configure/cstrings"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/planet/Godeps/_workspace/src/gopkg.in/alecthomas/kingpin.v2"
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
	ServiceSubnet           CIDR
	PODSubnet               CIDR
	InitialCluster          keyValueList
	ServiceUser             *user.User
	ServiceUID              string
	ServiceGID              string
	EtcdMemberName          string
	EtcdInitialCluster      string
	EtcdInitialClusterState string
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

func CIDRFlag(s kingpin.Settings) *CIDR {
	vars := new(CIDR)
	s.SetValue(vars)
	return vars
}

type CIDR struct {
	val   string
	ip    net.IP
	ipnet net.IPNet
}

func (c *CIDR) Set(v string) error {
	ip, ipnet, err := net.ParseCIDR(v)
	if err != nil {
		return err
	}
	c.val = v
	c.ip = ip
	c.ipnet = *ipnet
	return nil
}

func (c *CIDR) String() string {
	return c.ipnet.String()
}

// FirstIP returns the first IP in this subnet that is not .0
func (c *CIDR) FirstIP() net.IP {
	var ip net.IP
	for ip = IncIP(c.ip.Mask(c.ipnet.Mask)); c.ipnet.Contains(ip); IncIP(ip) {
		break
	}
	return ip
}

// RelativeIP returns an IP given an offset from the first IP in the range.
// offset starts at 0, i.e. c.RelativeIP(0) == c.FirstIP()
func (c *CIDR) RelativeIP(offset int) net.IP {
	var ip net.IP
	for ip = IncIP(c.ip.Mask(c.ipnet.Mask)); c.ipnet.Contains(ip) && offset > 0; IncIP(ip) {
		offset--
	}
	return ip
}

func IncIP(ip net.IP) net.IP {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
	return ip
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

// keyValueList is a command line flag that can extract
// key=value pair lists from the input:
//
// key=value,key2=value2,...
type keyValueList [][2]string

func (r *keyValueList) Set(input string) error {
	keyValues := strings.Split(input, ",")
	for _, keyValue := range keyValues {
		values := strings.SplitN(keyValue, "=", 2)
		if len(values) != 2 {
			return trace.Errorf("invalid key/value pair `%v` in `%v`", keyValue, input)
		}
		key, value := values[0], values[1]
		*r = append(*r, [2]string{key, value})
	}
	return nil
}

func (r keyValueList) String() string {
	var keyValues []string
	for _, pair := range r {
		key, value := pair[0], pair[1]
		keyValues = append(keyValues, fmt.Sprintf("%v=%v", key, value))
	}
	return strings.Join(keyValues, ",")
}
