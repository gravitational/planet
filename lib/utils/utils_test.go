package utils

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	. "gopkg.in/check.v1"
)

func TestUtils(t *testing.T) { TestingT(t) }

type UtilsSuite struct {
}

var _ = Suite(&UtilsSuite{})

func (s *UtilsSuite) TestHosts(c *C) {
	tcs := []struct {
		input    string
		expected string
		hostname string
		ip       string
	}{
		{
			input:    "",
			expected: "127.0.0.1 example.com\n",
			hostname: "example.com",
			ip:       "127.0.0.1",
		},
		{
			input:    "127.0.0.2 example.com",
			expected: "127.0.0.1 example.com\n",
			hostname: "example.com",
			ip:       "127.0.0.1",
		},
		{
			input: `# The following lines are desirable for IPv4 capable hosts
127.0.0.1       localhost
146.82.138.7    master.debian.org      master
127.0.3.4       example.com example

`,
			expected: `# The following lines are desirable for IPv4 capable hosts
127.0.0.1       localhost
146.82.138.7    master.debian.org      master
127.0.0.1 example.com example

`,
			hostname: "example.com",
			ip:       "127.0.0.1",
		},
	}
	tempDir := c.MkDir()
	for i, tc := range tcs {
		// test file
		comment := Commentf("test #%d (%v)", i+1)
		buf := &bytes.Buffer{}
		err := UpsertHostsLine(strings.NewReader(tc.input), buf, tc.hostname, tc.ip)
		c.Assert(err, IsNil, comment)
		c.Assert(buf.String(), Equals, tc.expected, comment)

		// test file
		testFile := filepath.Join(tempDir, fmt.Sprintf("test_case_%v", i+1))
		err = ioutil.WriteFile(testFile, []byte(tc.input), 0666)
		c.Assert(err, IsNil)

		err = UpsertHostsFile(tc.hostname, tc.ip, testFile)
		c.Assert(err, IsNil)
		out, err := ioutil.ReadFile(testFile)
		c.Assert(err, IsNil)
		c.Assert(string(out), Equals, tc.expected, comment)
	}
}
