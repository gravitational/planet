/*
Copyright 2019 Gravitational, Inc.

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
	check "gopkg.in/check.v1"
	"strings"
)

type CommandFlagSuite struct{}

var _ = check.Suite(&CommandFlagSuite{})

func (*CommandFlagSuite) TestEnvParse(c *check.C) {
	var cases = []struct {
		value    string
		expected EnvVars
		comment  string
	}{
		{
			value: "var=value",
			expected: EnvVars{
				{
					Name: "var",
					Val:  "value",
				},
			},
			comment: "simple value",
		},
		{
			value: `var="value1,value2"`,
			expected: EnvVars{
				{
					Name: "var",
					Val:  "value1,value2",
				},
			},
			comment: "comma-separated value",
		},
		{
			value: `var1="value1,value2",var2="value1=value2"`,
			expected: EnvVars{
				{
					Name: "var1",
					Val:  "value1,value2",
				},
				{
					Name: "var2",
					Val:  "value1=value2",
				},
			},
			comment: "multiple comma-separated values",
		},
		{
			value: `var="value1,value2;value3:"`,
			expected: EnvVars{
				{
					Name: "var",
					Val:  "value1,value2;value3:",
				},
			},
			comment: "value in quotes is not interpreted",
		},
		{
			value: `var1=value1,var2=value2`,
			expected: EnvVars{
				{
					Name: "var1",
					Val:  "value1",
				},
				{
					Name: "var2",
					Val:  "value2",
				},
			},
			comment: "multiple variables",
		},
		{
			value: `var1=value1,var2=value2,`,
			expected: EnvVars{
				{
					Name: "var1",
					Val:  "value1",
				},
				{
					Name: "var2",
					Val:  "value2",
				},
			},
			comment: "empty input ignored",
		},
		{
			value: `VAR1=VALUE1,var2=value2`,
			expected: EnvVars{
				{
					Name: "VAR1",
					Val:  "VALUE1",
				},
				{
					Name: "var2",
					Val:  "value2",
				},
			},
			comment: "allows upper and lower-case names",
		},
		{
			value: ` var1=value1, var2 = value2 `,
			expected: EnvVars{
				{
					Name: "var1",
					Val:  "value1",
				},
				{
					Name: "var2",
					Val:  "value2",
				},
			},
			comment: "allows whitespace around names/values",
		},
	}

	for _, tt := range cases {
		comment := check.Commentf(tt.comment)
		p := newEnvParser(tt.value)
		vars, err := p.parse()
		c.Assert(err, check.IsNil, comment)
		c.Assert(vars, check.DeepEquals, tt.expected, comment)
	}
}

func (r *CommandFlagSuite) TestEnvDelete(c *check.C) {
	var cases = []struct {
		add         string
		delete      string
		expected    string
		description string
	}{
		{
			add:         "alpha=1",
			expected:    "alpha=1",
			description: "add alpha",
		},
		{
			add:         "bravo=2",
			expected:    "alpha=1,bravo=2",
			description: "add bravo",
		},
		{
			add:         "charlie=3",
			expected:    "alpha=1,bravo=2,charlie=3",
			description: "add charlie",
		},
		{
			delete:      "bravo",
			expected:    "alpha=1,charlie=3",
			description: "delete bravo",
		},
		{
			delete:      "charlie",
			expected:    "alpha=1",
			description: "delete charlie",
		},
		{
			delete:      "alpha",
			expected:    "",
			description: "delete alpha",
		},
	}

	vars := EnvVars{}
	for _, tt := range cases {
		if len(tt.add) != 0 {
			split := strings.Split(tt.add, "=")
			vars.Upsert(split[0], split[1])
		}

		if len(tt.delete) != 0 {
			vars.Delete(tt.delete)
		}

		c.Assert(vars.String(), check.Equals, tt.expected)
	}
}
