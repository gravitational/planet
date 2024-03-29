/*
Copyright 2020 Gravitational, Inc.

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
	"fmt"
	"net"
	"strings"

	kv "github.com/gravitational/configure"
	"github.com/gravitational/trace"
	"gopkg.in/alecthomas/kingpin.v2"
)

func cidrFlag(s kingpin.Settings) *cidr {
	var c cidr
	s.SetValue(&c)
	return &c
}

// String returns the text representation of this subnet value
func (r *cidr) String() string {
	return r.ipNet.String()
}

// Set interprets the specified value as a network CIDR.
// Implements kingpin.Value
func (r *cidr) Set(value string) error {
	_, ipNet, err := net.ParseCIDR(value)
	if err != nil {
		return trace.ConvertSystemError(err)
	}
	r.ipNet = *ipNet
	return nil
}

type cidr struct {
	ipNet net.IPNet
}

// toAddrList interprets each key/value as domain=addr and extracts
// just the address part.
func toAddrList(store kv.KeyVal) (addrs []string) {
	for _, addr := range store {
		addrs = append(addrs, addr)
	}
	return addrs
}

// toEctdPeerList interprets each key/value pair as domain=addr,
// decorates each in etcd peer format.
func toEtcdPeerList(list kv.KeyVal) (peers string) {
	var addrs []string
	for domain, addr := range list {
		addrs = append(addrs, fmt.Sprintf("%v=https://%v:2380", domain, addr))
	}
	return strings.Join(addrs, ",")
}

// toEtcdGatewayList interprets each key/value pair, and
// formats it as a list of endpoints the etcd gateway can
// proxy to
func toEtcdGatewayList(list kv.KeyVal) (peers string) {
	var addrs []string
	for _, addr := range list {
		addrs = append(addrs, fmt.Sprintf("%v:2379", addr))
	}
	return strings.Join(addrs, ",")
}
