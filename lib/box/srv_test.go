package box

import (
	"bytes"
	"io"

	"gopkg.in/check.v1"
)

type SrvSuite struct{}

var _ = check.Suite(&SrvSuite{})

func (s *SrvSuite) TestWriteReadEnvironment(c *check.C) {
	expected := EnvVars{
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
			Val:  `first "second" third`,
		},
	}
	var buf bytes.Buffer
	_, err := io.Copy(&buf, expected)
	c.Assert(err, check.IsNil)

	env, err := ReadEnvironmentFromReader(&buf)
	c.Assert(err, check.IsNil)
	c.Assert(env, check.DeepEquals, expected)
}

func (vars EnvVars) Read(p []byte) (n int, err error) {
	return 0, io.EOF
}
