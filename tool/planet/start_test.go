package main

import (
	"testing"

	"github.com/gravitational/planet/lib/box"
	"github.com/gravitational/planet/lib/utils"
	"gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { check.TestingT(t) }

type StartSuite struct{}

var _ = check.Suite(&StartSuite{})

func (_ *StartSuite) TestCoreDNSConf(c *check.C) {
	dnsConfig := &Config{
		DNS: DNS{
			ListenAddrs: []string{"127.0.0.2", "127.0.0.3"},
			Port:        53,
			Zones: box.DNSOverrides{
				"example.com":  []string{"1.1.1.1", "2.2.2.2"},
				"example2.com": []string{"1.1.1.1", "2.2.2.2"},
			},
			Hosts: box.DNSOverrides{
				"override.com":  []string{"5.5.5.5", "7.7.7.7"},
				"override2.com": []string{"1.2.3.4"},
			},
		},
	}

	resolv := &utils.DNSConfig{
		Servers: []string{"1.1.1.1", "8.8.8.8"},
	}

	config, err := generateCoreDNSConfig(dnsConfig, resolv)

	c.Assert(err, check.IsNil)
	c.Assert(config, check.Equals, expectedCoreDnsConfig)
}

var expectedCoreDnsConfig = `
import /etc/coredns/configmap/*

.:53 {
  reload
  bind 127.0.0.2 127.0.0.3 
  errors
  hosts /etc/coredns/coredns.hosts { 
    5.5.5.5 override.com
    7.7.7.7 override.com
    1.2.3.4 override2.com
    fallthrough
  }
  kubernetes cluster.local in-addr.arpa ip6.arpa {
    endpoint https://leader.telekube.local:6443
    tls /var/state/apiserver-kubelet-client.cert /var/state/apiserver-kubelet-client.key /var/state/root.cert
    pods disabled
  }
  proxy example.com 1.1.1.1 2.2.2.2 {
    policy sequential
  }
  proxy example2.com 1.1.1.1 2.2.2.2 {
    policy sequential
  }
  forward . 1.1.1.1 8.8.8.8 {
    policy sequential
    health_check 0
  }
}
`
