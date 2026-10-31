package jsonpath

import (
	"testing"
	"time"
)

// FuzzParseQueryString checks that the query parser never panics, whatever
// the input.
func FuzzParseQueryString(f *testing.F) {
	for _, seed := range []string{
		"$", "$.a", "$..a", "$[*]", "$[0, -1, 1:2:3, 'a', \"b\"]", "$[?@.a == 1 && !(@.b < 2) || @.c]",
		"$[?match(@.a, 'x.*')]", "$[?length(@) > 1]", "$[?count($..a) == value(@.b)]", "$['\\u0041']",
		"$[", "$.", "@.a", "$[?]", "$[?@.a ==]", "$[1:2:0]", "$[-0]", "$[9999999999999999999]", "$..", "$.a..b",
		"$[?@.a == 1.5e300]", "$[?@ == 'a\\'b']", "", "  $  ", "$[?@.a == \"\\ud800\"]",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, query string) {
		start := time.Now()
		ParseQueryString(query)
		ParseQueryStringStrict(query)
		// Parsing must be fast whatever the input: a query is a few dozen
		// bytes typed on a command line.
		if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
			t.Fatalf("parsing %q took %s", query, elapsed)
		}
	})
}
