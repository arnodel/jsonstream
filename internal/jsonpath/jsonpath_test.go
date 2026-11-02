package jsonpath

import (
	"strings"
	"testing"
	"time"
)

func TestParseQueryStringInvalid(t *testing.T) {
	invalid := []string{
		// Queries missing the leading root identifier fail in grammar.Parse.
		"", "@.x", ".x", "[0]",
		// Queries with trailing garbage or unterminated brackets.
		"$[", "$]", "$$", "$[?@.x >]",
	}
	for _, s := range invalid {
		if _, err := ParseQueryString(s); err == nil {
			t.Errorf("ParseQueryString(%q): expected an error, got nil", s)
		}
	}
}

func TestParseQueryStringValid(t *testing.T) {
	for _, s := range []string{"$", "$.a", "$..b[0]", "$[?@.x > 1]"} {
		if _, err := ParseQueryString(s); err != nil {
			t.Errorf("ParseQueryString(%q): unexpected error: %v", s, err)
		}
	}
}

// TestParseDeepNestingIsFast checks that parsing time does not multiply with
// nesting.  Several alternatives of the grammar can start with a function
// call or a filter query, and without remembering failures (see
// parser/memo.go) each level re-parsed the ones below it several times over:
// ten unclosed function calls took seconds, thirty would take years.  The
// limit is generous so the test is not flaky; these parse in about a
// millisecond.
func TestParseDeepNestingIsFast(t *testing.T) {
	const depth = 30
	for _, prefix := range []string{"a(", "a(@,", "a(1,", "@[?a(", "a(!", "(", "@[?("} {
		for _, suffix := range []string{"", "1 ==", "1"} {
			for _, closed := range []bool{false, true} {
				query := "$[?" + strings.Repeat(prefix, depth) + suffix
				if closed {
					query += strings.Repeat(")", depth) + "]"
				}
				start := time.Now()
				ParseQueryString(query)
				ParseQueryStringStrict(query)
				if elapsed := time.Since(start); elapsed > time.Second {
					t.Errorf("parsing %q took %s", query, elapsed)
				}
			}
		}
	}
}
