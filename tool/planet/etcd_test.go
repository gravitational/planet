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
	"io/ioutil"
	"os"
	"testing"

	"github.com/gravitational/trace"
	check "gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { check.TestingT(t) }

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
