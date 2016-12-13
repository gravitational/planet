package main

import (
	"os"
	"testing"

	q "gopkg.in/check.v1"
)

func TestStart(t *testing.T) { q.TestingT(t) }

type StartSuite struct{}

var _ = q.Suite(&StartSuite{})

func (r *StartSuite) TestReadsOSRelease(c *q.C) {
	file, err := os.Open("/etc/os-release")
	c.Assert(err, q.IsNil)
	defer file.Close()

	id, err := getSystemID(file)
	c.Assert(err, q.IsNil)
	c.Logf("system ID: %v", id)
}
