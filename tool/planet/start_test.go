package main

import (
	"testing"

	"gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { check.TestingT(t) }

type StartSuite struct{}

var _ = check.Suite(&StartSuite{})

func (_ *StartSuite) TestCoreDNSConf(c *check.C) {
	var configTable = []struct {
		config   corednsConfig
		expected string
	}{
		{
			corednsConfig{
				ListenAddrs: []string{"127.0.0.2", "127.0.0.3"},
				Port:        53,
				Zones: map[string][]string{
					"example.com":  []string{"1.1.1.1", "2.2.2.2"},
					"example2.com": []string{"1.1.1.1", "2.2.2.2"},
				},
				Hosts: map[string][]string{
					"override.com":  []string{"5.5.5.5", "7.7.7.7"},
					"override2.com": []string{"1.2.3.4"},
				},
				UpstreamNameservers: []string{"1.1.1.1", "8.8.8.8"},
				Import:              true,
			},
			`
import /etc/coredns/configmaps/*
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
    tls /var/state/coredns.cert /var/state/coredns.key /var/state/root.cert
    pods verified
    fallthrough in-addr.arpa ip6.arpa
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
`,
		},
		{
			corednsConfig{
				Port:                55,
				ListenAddrs:         []string{"127.0.0.2"},
				UpstreamNameservers: []string{"1.1.1.1"},
				Rotate:              true,
				Import:              true,
			},
			`
import /etc/coredns/configmaps/*
.:55 {
  reload
  bind 127.0.0.2 
  errors
  hosts /etc/coredns/coredns.hosts { 
    fallthrough
  }
  kubernetes cluster.local in-addr.arpa ip6.arpa {
    endpoint https://leader.telekube.local:6443
    tls /var/state/coredns.cert /var/state/coredns.key /var/state/root.cert
    pods verified
    fallthrough in-addr.arpa ip6.arpa
  }
  forward . 1.1.1.1 {
    policy random
    health_check 0
  }
}
`,
		},
	}

	for _, tt := range configTable {
		config, err := generateCorednsConfig(tt.config, corednsTemplate)

		c.Assert(err, check.IsNil)
		c.Assert(config, check.Equals, tt.expected)
	}

}
