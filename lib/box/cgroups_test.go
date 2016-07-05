package box

import (
	"testing"

	. "gopkg.in/check.v1"
)

func TestCgroups(t *testing.T) { TestingT(t) }

type CGSuite struct {
}

var _ = Suite(&CGSuite{})

func (s *CGSuite) TestParsing(c *C) {

}
