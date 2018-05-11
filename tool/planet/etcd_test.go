package main

import (
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

func (*EtcdSuite) TestEtcdParseFile(c *check.C) {
	file, err := ioutil.TempFile(os.TempDir(), "prefix")
	c.Assert(err, check.IsNil)
	defer os.Remove(file.Name())

	// reading a missing file should return an error
	_, _, err = readEtcdVersion("/this/file/doesnt/exist")
	c.Assert(trace.IsNotFound(err), check.Equals, true)

	// try writing the etcd environment
	current := "v1.1.1"
	prev := "v1.0.0"
	err = writeEtcdEnvironment(file.Name(), current, prev)
	c.Assert(err, check.IsNil)

	cu, pr, err := readEtcdVersion(file.Name())
	c.Assert(err, check.IsNil)
	c.Assert(cu, check.Equals, current)
	c.Assert(pr, check.Equals, prev)
}
