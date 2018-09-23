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
		config   coreDNSConfig
		expected string
	}{
		{
			coreDNSConfig{
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
			},
			`
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
			coreDNSConfig{
				Port:                55,
				ListenAddrs:         []string{"127.0.0.2"},
				UpstreamNameservers: []string{"1.1.1.1"},
				Rotate:              true,
			},
			`
import /etc/coredns/configmap/*

.:55 {
  reload
  bind 127.0.0.2 
  errors
  hosts /etc/coredns/coredns.hosts { 
    fallthrough
  }
  kubernetes cluster.local in-addr.arpa ip6.arpa {
    endpoint https://leader.telekube.local:6443
    tls /var/state/apiserver-kubelet-client.cert /var/state/apiserver-kubelet-client.key /var/state/root.cert
    pods disabled
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
		config, err := generateCoreDNSConfig(tt.config)

		c.Assert(err, check.IsNil)
		c.Assert(config, check.Equals, tt.expected)
	}

}
