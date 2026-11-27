package token

import (
	"bytes"
	"encoding/json"
	"math"
	"testing"
)

// oracle parses a JSON scalar the slow way, with the generic decoder, which
// is what parseJsonLiteralBytes used to do.
func oracle(b []byte) any {
	dec := json.NewDecoder(bytes.NewReader(b))
	tok, err := dec.Token()
	if err != nil {
		panic(err)
	}
	return tok
}

var literalCorpus = []string{
	`0`, `-0`, `1`, `-1`, `42`, `3.14`, `-2.5e3`, `1E-7`, `1e308`, `123456789012345678901234567890`,
	`""`, `"a"`, `"hello world"`, `"with \"quotes\""`, `"back\\slash"`, `"slash\/"`,
	`"tab\tnl\n"`, `"\u00e9t\u00e9"`, `"\ud83d\ude00"`, `"mixed \u00e9 and é"`, `"\u0000"`,
	`true`, `false`, `null`,
}

func TestParseLiteralBytesMatchesDecoder(t *testing.T) {
	for _, lit := range literalCorpus {
		b := []byte(lit)
		got, want := ParseLiteralBytes(b), oracle(b)
		if got != want {
			t.Errorf("%s: got %#v (%T), want %#v (%T)", lit, got, got, want, want)
		}
	}
}

func TestParseLiteralBytesOutOfRange(t *testing.T) {
	// The generic decoder gives +Inf for numbers beyond float64; keep that.
	got := ParseLiteralBytes([]byte(`1e999`))
	if got != math.Inf(1) {
		t.Errorf("1e999: got %v, want +Inf", got)
	}
}

func TestScalarToFloat64AndToString(t *testing.T) {
	if got := NewScalar(Number, []byte(`-12.5e1`)).ToFloat64(); got != -125 {
		t.Errorf("ToFloat64: got %v", got)
	}
	if got := NewScalar(String, []byte(`"caf\u00e9"`)).ToString(); got != "café" {
		t.Errorf("ToString escaped: got %q", got)
	}
	unescaped := NewScalar(String, []byte(`"plain"`))
	unescaped.TypeAndFlags |= UnescapedMask
	if got := unescaped.ToString(); got != "plain" {
		t.Errorf("ToString unescaped: got %q", got)
	}
	defer func() {
		if recover() == nil {
			t.Error("ToFloat64 on a string should panic")
		}
	}()
	NewScalar(String, []byte(`"1"`)).ToFloat64()
}

func TestScalarEqualEscapedStrings(t *testing.T) {
	a := NewScalar(String, []byte(`"café"`))
	b := NewScalar(String, []byte(`"caf\u00e9"`))
	if !a.Equal(b) || !b.Equal(a) {
		t.Error("escaped and raw forms of the same string should be equal")
	}
	c := NewScalar(Number, []byte(`1e2`))
	d := NewScalar(Number, []byte(`100`))
	if !c.Equal(d) {
		t.Error("1e2 and 100 should be equal")
	}
}

// A string has the same value whether or not it has escapes: they are decoded,
// and every other byte is kept, invalid UTF-8 included.  Only an escaped
// surrogate that is not part of a pair has no value of its own: it is U+FFFD.
func TestUnquote(t *testing.T) {
	for _, c := range []struct{ lit, want string }{
		{`"plain"`, "plain"},
		{"\"\xb9\"", "\xb9"},
		{"\"a\\n\xb9\"", "a\n\xb9"},
		{`"\u0041\t\\\"\/"`, "A\t\\\"/"},
		{`"\ud83d\ude00"`, "😀"},
		{`"\uD83D\uDE00"`, "😀"},
		{`"\ud800"`, "\ufffd"},
		{`"\ud83dx"`, "\ufffdx"},
		{`"\ud83d\u0041"`, "\ufffdA"},
		{`"\ude00\ud83d"`, "\ufffd\ufffd"},
	} {
		if got := unquote([]byte(c.lit)); got != c.want {
			t.Errorf("%s: got %q, want %q", c.lit, got, c.want)
		}
	}
}

// Found by FuzzJPVRoundTrip: an unescaped key of invalid UTF-8 kept its bytes,
// while an escaped one was decoded with invalid UTF-8 replaced, so "\xb9" was
// equal to "\ud800" when compared with Equal but not with ToString.
func TestScalarEqualInvalidUTF8(t *testing.T) {
	raw := NewScalar(String, []byte("\"\xb9\""))
	unescaped := NewScalar(String, []byte("\"\xb9\""))
	unescaped.TypeAndFlags |= UnescapedMask
	surrogate := NewScalar(String, []byte(`"\ud800"`))
	if raw.ToString() != unescaped.ToString() {
		t.Error("the unescaped flag changed the value of a string")
	}
	for _, s := range []*Scalar{raw, unescaped} {
		if s.Equal(surrogate) || surrogate.Equal(s) {
			t.Errorf("%q should not equal %s", s.Bytes, surrogate.Bytes)
		}
	}
}
