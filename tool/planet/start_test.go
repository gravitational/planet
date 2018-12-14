/*
Copyright 2018 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
			coreDNSConfig{
				Port:                55,
				ListenAddrs:         []string{"127.0.0.2"},
				UpstreamNameservers: []string{"1.1.1.1"},
				Rotate:              true,
			},
			`
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
		config, err := generateCoreDNSConfig(tt.config, coreDNSTemplate)

		c.Assert(err, check.IsNil)
		c.Assert(config, check.Equals, tt.expected)
	}

}
