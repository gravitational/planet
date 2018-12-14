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

package box

import (
	"io/ioutil"
	"os"

	"gopkg.in/check.v1"
)

type SrvSuite struct{}

var _ = check.Suite(&SrvSuite{})

func (s *SrvSuite) TestWriteReadEnvironment(c *check.C) {
	f, err := ioutil.TempFile("", "")
	c.Assert(err, check.IsNil)
	defer os.Remove(f.Name())

	err = WriteEnvironment(f.Name(), EnvVars{
		{
			Name: "KUBE_MASTER_IP",
			Val:  "192.168.122.176",
		},
		{
			Name: "DOCKER_OPTS",
			Val:  "--storage-driver=devicemapper --exec-opt native.cgroupdriver=cgroupfs",
		},
		{
			Name: "EMPTY_VAR",
			Val:  "",
		},
		{
			Name: "WITH_QUOTES",
			Val:  `blah "blah" blah`,
		},
	})
	c.Assert(err, check.IsNil)

	env, err := ReadEnvironment(f.Name())
	c.Assert(err, check.IsNil)
	c.Assert(env.Get("KUBE_MASTER_IP"), check.Equals, "192.168.122.176")
	c.Assert(env.Get("DOCKER_OPTS"), check.Equals,
		"--storage-driver=devicemapper --exec-opt native.cgroupdriver=cgroupfs")
	c.Assert(env.Get("EMPTY_VAR"), check.Equals, "")
	c.Assert(env.Get("WITH_QUOTES"), check.Equals, `blah "blah" blah`)
}
