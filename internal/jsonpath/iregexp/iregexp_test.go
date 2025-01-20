package iregexp

import (
	"testing"
)

func TestIRegexpString(t *testing.T) {
	type testCase struct {
		name string
		ire  string
		re   string
	}
	var testCases = []testCase{
		{
			name: "no dot",
			ire:  "hello",
			re:   "hello",
		},
		{
			name: "simple dot",
			ire:  ".",
			re:   `[^\n\r]`,
		},
		{
			name: "dot in character class",
			ire:  `[a-z.]`,
			re:   `[a-z.]`,
		},
		{
			name: "dot in and out of character class",
			ire:  `[a-z.].`,
			re:   `[a-z.][^\n\r]`,
		},
		{
			name: "escaped dot",
			ire:  `ab\.`,
			re:   `ab\.`,
		},
		{
			name: "escaped square brackets",
			ire:  `\[a-z.\]`,
			re:   `\[a-z[^\n\r]\]`,
		},
		{
			name: "two character classes",
			ire:  `[a-z.].[^xy].`,
			re:   `[a-z.][^\n\r][^xy][^\n\r]`,
		},
		{
			name: "opening square bracket in character class",
			ire:  `[a[b.].`,
			re:   `[a[b.][^\n\r]`,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			re := iregexpString(c.ire)
			if re != c.re {
				t.Errorf("Expected %q, got %q", c.re, re)
			}
		})
	}
}
