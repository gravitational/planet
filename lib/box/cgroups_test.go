package box

import (
	"strings"
	"testing"

	. "gopkg.in/check.v1"
)

func TestCgroups(t *testing.T) { TestingT(t) }

type CGSuite struct {
}

var _ = Suite(&CGSuite{})

func (s *CGSuite) TestParsing(c *C) {
	tcs := []struct {
		groups   string
		expected map[string]bool
	}{

		{
			groups:   ``,
			expected: map[string]bool{},
		},
		{
			groups: `#subsys_name	hierarchy	num_cgroups	enabled
cpuset	2	1	1
cpu	3	1	1
cpuacct	3	1	1
memory	0	1	0
devices	4	1	1
freezer	5	1	1
net_cls	6	1	1
blkio	7	1	1
perf_event	8	1	1
net_prio	6	1	1
`,
			expected: map[string]bool{
				"net_cls,net_prio": true,
				"blkio":            true,
				"perf_event":       true,
				"cpuset":           true,
				"cpu,cpuacct":      true,
				"devices":          true,
				"freezer":          true,
			},
		},
	}
	for i, tc := range tcs {
		comment := Commentf("test #%d (%v)", i+1)
		set, err := parseCgroups(strings.NewReader(tc.groups))
		c.Assert(err, IsNil, comment)
		c.Assert(set, DeepEquals, tc.expected, comment)
	}
}
