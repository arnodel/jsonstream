package jsonpath_test

import (
	"testing"
)

// These exercise the compile-time literal paths: comparisons with a literal
// operand and match()/search() with a literal pattern.  The compliance suite
// covers the semantics broadly; these pin down the cases where the fast paths
// differ from the generic ones in implementation.
func TestLiteralComparisons(t *testing.T) {
	const doc = `[
		{"n": 100, "s": "café", "b": true, "z": null, "o": {"x": 1}},
		{"n": 1e2, "s": "café", "b": false, "z": 0},
		{"n": 99.5, "s": "cafe", "b": true},
		{"n": "100", "s": 100}
	]`
	cases := []struct{ query, want string }{
		// Numbers: equal under different spellings, ordering, literal on either side.
		{`$[?@.n == 100].s`, `"café" "café"`},
		{`$[?100 == @.n].s`, `"café" "café"`},
		{`$[?@.n < 100].s`, `"cafe"`},
		{`$[?100 > @.n].s`, `"cafe"`},
		{`$[?@.n <= 100].s`, `"café" "café" "cafe"`},
		{`$[?@.n >= 100].s`, `"café" "café"`},
		{`$[?@.n != 100].s`, `"cafe" 100`},
		// Strings: escaped and raw spellings are equal; ordering is bytewise.
		{`$[?@.s == "café"].n`, `100 1e2`},
		{`$[?@.s == "café"].n`, `100 1e2`},
		{`$[?@.s < "café"].n`, `99.5`},
		{`$[?"cafe" < @.s].n`, `100 1e2`},
		// Booleans and null: equality only, never ordered.
		{`$[?@.b == true].n`, `100 99.5`},
		{`$[?@.b == false].n`, `1e2`},
		{`$[?@.z == null].n`, `100`},
		{`$[?@.z < 1].n`, `1e2`},
		{`$[?@.b < true].n`, ``},
		// A missing node or a non-scalar node never equals a literal, and
		// != is true for them.
		{`$[?@.missing == 1].n`, ``},
		{`$[?@.missing != 1].n`, `100 1e2 99.5 "100"`},
		{`$[?@.o == 1].n`, ``},
		{`$[?@.o != 1].n`, `100 1e2 99.5 "100"`},
		// Different types never compare.
		{`$[?@.n == "100"].n`, `"100"`},
		{`$[?@.n < "200"].n`, `"100"`},
		// Literal on both sides is still handled (generic path).
		{`$[?1 < 2].n`, `100 1e2 99.5 "100"`},
	}
	for _, c := range cases {
		t.Run(c.query, func(t *testing.T) {
			if got := collectScalars(t, c.query, jsonInput(doc)); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestLiteralRegexFunctions(t *testing.T) {
	const doc = `[
		{"a": "ab"}, {"a": "a"}, {"a": "abc"}, {"a": "xaby"}, {"a": "café"}, {"a": 12}, {"b": "ab"}
	]`
	cases := []struct{ query, want string }{
		// match() requires the whole string to match: a|ab on "ab" is true.
		{`$[?match(@.a, "a|ab")].a`, `"ab" "a"`},
		{`$[?match(@.a, "a.*")].a`, `"ab" "a" "abc"`},
		{`$[?search(@.a, "ab")].a`, `"ab" "abc" "xaby"`},
		// Escaped subject.
		{`$[?match(@.a, "caf.")].a`, `"café"`},
		// Non-string subject or missing node: false.
		{`$[?match(@.a, ".*")].a`, `"ab" "a" "abc" "xaby" "café"`},
		{`$[?match(@.b, ".*")].b`, `"ab"`},
		// An invalid pattern is false for every node, and does not panic on
		// the second node.
		{`$[?match(@.a, "(")].a`, ``},
		{`$[?search(@.a, "(")].a`, ``},
		// Negation.
		{`$[?!search(@.a, "ab")].a`, `"a" "café" 12`},
		// Non-literal pattern uses the generic path, which must agree.
		{`$[?match(@.a, $[0].a)].a`, `"ab"`},
	}
	for _, c := range cases {
		t.Run(c.query, func(t *testing.T) {
			if got := collectScalars(t, c.query, jsonInput(doc)); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// TestRootQueryOnScalarDocument checks queries referring to the root ($) in a
// filter when the document itself is a scalar (found by FuzzQuery: the
// scalar's Clone has no detach function, which was called unconditionally).
func TestRootQueryOnScalarDocument(t *testing.T) {
	for _, c := range []struct{ query, doc, want string }{
		{`$[?$ != 'x']`, `0`, ``},
		{`$[?@ == $]`, `[1, 2]`, ``},
		{`$[?@ < $[1]]`, `[1, 2]`, `1`},
		{`$[?count($[*]) == 2]`, `[1, 2]`, `1 2`},
		{`$[?count($[*]) == 2]`, `"s"`, ``},
		// A root singular query that finds nothing (the root is an array, so
		// $.a does not exist) compares as Nothing: equal to a missing node,
		// different from everything else.
		{`$[?$.a == @.b]`, `[{"b": 1}, {"c": 2}]`, `"c" 2`},
		{`$[?$.a != @.b]`, `[{"b": 1}, {"c": 2}]`, `"b" 1`},
		{`$[?$[5] == 1]`, `[1, 2]`, ``},
	} {
		if got := collectScalars(t, c.query, jsonInput(c.doc)); got != c.want {
			t.Errorf("%s on %s: got %q, want %q", c.query, c.doc, got, c.want)
		}
	}
}
