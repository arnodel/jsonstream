package jsonpath

import "testing"

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
