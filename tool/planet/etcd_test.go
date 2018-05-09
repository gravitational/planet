package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/gravitational/trace"
	check "gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func TestEtcd(t *testing.T) { check.TestingT(t) }

type EtcdSuite struct{}

var _ = check.Suite(&EtcdSuite{})

func (*EtcdSuite) TestEtcdAssumedVersion(c *check.C) {
	ver, err := currentEtcdVersion("/this/file/doesnt/exist")
	c.Assert(trace.IsNotFound(err), check.Equals, true)
	c.Assert(err, check.IsNil)
	c.Assert(ver, check.Equals, AssumeEtcdVersion)
}

func (*EtcdSuite) TestEtcdParseFile(c *check.C) {
	file, err := ioutil.TempFile(os.TempDir(), "prefix")
	c.Assert(err, IsNil)
	defer os.Remove(file.Name())

	// reading an empty file should return an error
	ver, err := currentEtcdVersion(file.Name())
	c.Assert(err, check.NotNil)
	c.Assert(ver, check.Equals, "")

	// reading a missing file should return an error
	ver, err = readEtcdVersion("/this/file/doesnt/exist")
	c.Assert(err, check.NotNil)
	c.Assert(ver, check.Equals, "")

	// write a version file then check it
	version := "1.1.1"
	fmt.Fprintf(file, "%v=%v", EnvEtcdVersion, version)
	file.Sync()

	ver, err = currentEtcdVersion(file.Name())
	c.Assert(err, check.IsNil)
	c.Assert(ver, check.Equals, version)
}
