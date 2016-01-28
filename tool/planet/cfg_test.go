package main

import (
	"testing"

	check "github.com/gravitational/planet/Godeps/_workspace/src/gopkg.in/check.v1"
)

func TestCommandFlags(t *testing.T) { check.TestingT(t) }

type CommandFlagSuite struct{}

var _ = check.Suite(&CommandFlagSuite{})

func (r *CommandFlagSuite) TestInputsKeyValues(c *check.C) {
	input := `172.168.178.1.example.com=172.168.178.2,172.168.178.1.example.com=172.168.178.3`

	// exercise
	var result keyValueList
	err := result.Set(input)

	// validate
	c.Assert(err, check.IsNil)
	c.Assert(len(result), check.Equals, 2)
}

func (r *CommandFlagSuite) TestExtractsAddr(c *check.C) {
	input := `172.168.178.1.example.com=172.168.178.1,172.168.178.2.example.com=172.168.178.2`

	// exercise
	var result keyValueList
	result.Set(input)
	addrs := toAddrList(result)

	// validate
	expected := []string{"172.168.178.1", "172.168.178.2"}
	c.Assert(len(addrs), check.Equals, 2)
	c.Assert(addrs, check.DeepEquals, expected)
}

func (r *CommandFlagSuite) TestConvertsToEtcdPeer(c *check.C) {
	input := `172.168.178.1.example.com=172.168.178.1,172.168.178.2.example.com=172.168.178.2`

	// exercise
	var result keyValueList
	result.Set(input)
	addrs := toEtcdPeerList(result)

	// validate
	expected := "172.168.178.1.example.com=http://172.168.178.1:2380,172.168.178.2.example.com=http://172.168.178.2:2380"
	c.Assert(addrs, check.DeepEquals, expected)
}
