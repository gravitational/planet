package pkg

import (
	"testing"

	. "gopkg.in/check.v1"
)

func TestLocators(t *testing.T) { TestingT(t) }

type LocatorSuite struct {
}

var _ = Suite(&LocatorSuite{})

func (s *LocatorSuite) TestLocatorOK(c *C) {
	tcs := []struct {
		loc  string
		name string
		ver  string
	}{
		{
			loc:  "example.com:0.0.1",
			name: "example.com",
			ver:  "0.0.1",
		},
		{
			loc:  "example0-9A.com/path:0.0.2",
			name: "example0-9A.com/path",
			ver:  "0.0.2",
		},
	}
	for i, tc := range tcs {
		comment := Commentf("test #%d (%v) loc=%v", i+1, tc.loc)
		loc, err := ParseLocator(tc.loc)
		c.Assert(err, IsNil, comment)
		c.Assert(loc.Repo, Equals, tc.name, comment)
		c.Assert(loc.Ver, Equals, tc.ver, comment)
	}
}

func (s *LocatorSuite) TestLocatorFail(c *C) {
	tcs := []string{
		"example.com:blabla", // not a sem ver
		"example .com:0.0.2", // unallowed chars
		"",                   //emtpy
		"arffewfaef aefeafaesf e", //garbage
		"-:.",
	}
	for i, tc := range tcs {
		comment := Commentf("test #%d (%v) loc=%v", i+1, tc)
		_, err := ParseLocator(tc)
		c.Assert(err, NotNil, comment)
	}
}
