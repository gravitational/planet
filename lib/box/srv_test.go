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
	})
	c.Assert(err, check.IsNil)

	env, err := ReadEnvironment(f.Name())
	c.Assert(err, check.IsNil)
	c.Assert(env.Get("KUBE_MASTER_IP"), check.Equals, "192.168.122.176")
	c.Assert(env.Get("DOCKER_OPTS"), check.Equals,
		"--storage-driver=devicemapper --exec-opt native.cgroupdriver=cgroupfs")
	c.Assert(env.Get("EMPTY_VAR"), check.Equals, "")
}
