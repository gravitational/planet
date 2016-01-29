package main

import (
	"fmt"
	"sort"
	"testing"

	kv "github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/configure"
	check "github.com/gravitational/planet/Godeps/_workspace/src/gopkg.in/check.v1"
)

func TestCommandFlags(t *testing.T) { check.TestingT(t) }

type CommandFlagSuite struct{}

var _ = check.Suite(&CommandFlagSuite{})

func (r *CommandFlagSuite) TestExtractsAddr(c *check.C) {
	input := `172.168.178.1.example.com:172.168.178.1,172.168.178.2.example.com:172.168.178.2`

	// exercise
	var result kv.KeyVal
	result.Set(input)
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
	result.Set(input)
	addrs := toEtcdPeerList(result)

	// validate
	expected := "172.168.178.1.example.com=http://172.168.178.1:2380,172.168.178.2.example.com=http://172.168.178.2:2380"
	expectedReverse := "172.168.178.2.example.com=http://172.168.178.2:2380,172.168.178.1.example.com=http://172.168.178.1:2380"
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
	return actual == expected[0] || actual == expected[1], ""
}
