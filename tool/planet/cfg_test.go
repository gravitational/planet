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
	"fmt"
	"sort"

	kv "github.com/gravitational/configure"
	check "gopkg.in/check.v1"
)

type CommandFlagSuite struct{}

var _ = check.Suite(&CommandFlagSuite{})

func (r *CommandFlagSuite) TestExtractsAddr(c *check.C) {
	input := `172.168.178.1.example.com:172.168.178.1,172.168.178.2.example.com:172.168.178.2`

	// exercise
	var result kv.KeyVal
	c.Assert(result.Set(input), check.IsNil)
	addrs := toAddrList(result)
	sort.Strings(addrs)

	// validate
	expected := []string{"172.168.178.1", "172.168.178.2"}
	sort.Strings(expected)
	c.Assert(addrs, check.HasLen, len(expected))
	c.Assert(addrs, check.DeepEquals, expected)
}

func (r *CommandFlagSuite) TestConvertsToEtcdPeer(c *check.C) {
	input := `172.168.178.1.example.com:172.168.178.1,172.168.178.2.example.com:172.168.178.2`

	// exercise
	var result kv.KeyVal
	c.Assert(result.Set(input), check.IsNil)
	addrs := toEtcdPeerList(result)

	// validate
	expected := "172.168.178.1.example.com=https://172.168.178.1:2380,172.168.178.2.example.com=https://172.168.178.2:2380"
	expectedReverse := "172.168.178.2.example.com=https://172.168.178.2:2380,172.168.178.1.example.com=https://172.168.178.1:2380"
	c.Assert(addrs, OneOfEquals, []string{expected, expectedReverse})
}

// oneOfChecker implements a gocheck.Checker that asserts that the actual value
// matches one of the values from the expected list.
type oneOfChecker struct {
	*check.CheckerInfo
}

var OneOfEquals check.Checker = &oneOfChecker{
	&check.CheckerInfo{Name: "OneOfEquals", Params: []string{"obtained", "expectedAlternatives"}},
}

func (r *oneOfChecker) Check(params []interface{}, names []string) (result bool, error string) {
	defer func() {
		if v := recover(); v != nil {
			result = false
			error = fmt.Sprint(v)
		}
	}()
	actual := params[0].(string)
	expected := params[1].([]string)
	result = actual == expected[0] || actual == expected[1]
	if !result {
		return false, "No match"
	}
	return true, ""
}
