package jpv

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/arnodel/jsonstream/encoding/json"
	"github.com/arnodel/jsonstream/format"
	"github.com/arnodel/jsonstream/token"
)

func fuzzDecodeJSON(input string) ([]token.Token, error) {
	acc := token.NewAccumulatorStream()
	err := json.NewDecoder(strings.NewReader(input)).Produce(acc)
	return acc.GetTokens(), err
}

func fuzzDecodeJPV(input string) ([]token.Token, error) {
	acc := token.NewAccumulatorStream()
	err := NewDecoder(strings.NewReader(input)).Produce(acc)
	return acc.GetTokens(), err
}

func fuzzEncodeJPV(toks []token.Token, quoteKeys bool) string {
	var buf bytes.Buffer
	enc := &Encoder{Printer: &format.DefaultPrinter{Writer: &buf, IndentSize: 0}, AlwaysQuoteKeys: quoteKeys}
	if err := enc.Consume(token.NewSliceReadStream(toks)); err != nil {
		panic(err)
	}
	return buf.String()
}

func fuzzTokensEqual(a, b []token.Token) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].String() != b[i].String() {
			return false
		}
	}
	return true
}

// FuzzJPVDecoder checks that the JPV decoder never panics on any input.
func FuzzJPVDecoder(f *testing.F) {
	for _, seed := range []string{
		"$ = 1\n", "$.a = 1\n$.b = [2]\n", `$["a b"][0] = "x"` + "\n", "$.a[0] = 1\n$.a[1] = 2\n$.b.c = null\n",
		"$.a = {}\n$.b = []\n", "$[0 = 1\n", "$.a\n", "= 1\n", "$.a = \n", "$.a.b = 1\n$.a = 2\n", "$[1] = 1\n$[0] = 2\n",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		fuzzDecodeJPV(input) // must not panic; errors are fine
	})
}

// FuzzJPVRoundTrip checks the README's claim that JSON -> JPV -> JSON loses
// nothing: for any valid JSON input, encoding it as JPV and decoding that
// gives back the same tokens.
func FuzzJPVRoundTrip(f *testing.F) {
	for _, seed := range []string{
		`{"a": 1, "b": [2, {"c": "x"}], "d": {}, "e": []}`, `[[], {}, [[]], [{}]]`, `{"": 1, "a b": 2, "a.b": 3, "\"": 4, "é": 5}`,
		`{"a": [{"b": []}, {"b": [1]}]}`, `[1]`, `"s"`, `null`, `{"1": {"2": [3]}}`, `{"$": 1, "[": 2, "]": 3, "=": 4}`,
		`{"a": {"a": {"a": null}}}`, `[[[[[1]]]]]`, `{"a": 1}` + "\n" + `{"a": 2}`,
	} {
		f.Add(seed, false)
		f.Add(seed, true)
	}
	f.Fuzz(func(t *testing.T, input string, quoteKeys bool) {
		toks, err := fuzzDecodeJSON(input)
		if err != nil || len(toks) == 0 {
			return
		}
		// JPV cannot tell a repeated path apart from a new document or a
		// duplicate key, so the property is for a single document whose
		// objects have distinct keys.
		if countValues(toks) != 1 || hasDuplicateKeys(toks) {
			return
		}
		jpv := fuzzEncodeJPV(toks, quoteKeys)
		again, err := fuzzDecodeJPV(jpv)
		if err != nil {
			t.Fatalf("decoding JPV failed: %v\nJSON: %q\nJPV:\n%s", err, input, jpv)
		}
		if !fuzzTokensEqual(toks, again) {
			t.Fatalf("JSON -> JPV -> JSON changed the value\nJSON: %q\nJPV:\n%s\ngot back: %v", input, jpv, again)
		}
	})
}

// countValues returns the number of top-level values in the token stream.
func countValues(toks []token.Token) int {
	n, depth := 0, 0
	for _, tok := range toks {
		switch tok.(type) {
		case *token.StartArray, *token.StartObject:
			if depth == 0 {
				n++
			}
			depth++
		case *token.EndArray, *token.EndObject:
			depth--
		case *token.Scalar:
			if depth == 0 {
				n++
			}
		}
	}
	return n
}

// hasDuplicateKeys reports whether any object in the stream repeats a key.
func hasDuplicateKeys(toks []token.Token) bool {
	var stack []map[string]bool // one set of keys per open object, nil for arrays
	for _, tok := range toks {
		switch x := tok.(type) {
		case *token.StartObject:
			stack = append(stack, map[string]bool{})
		case *token.StartArray:
			stack = append(stack, nil)
		case *token.EndArray, *token.EndObject:
			stack = stack[:len(stack)-1]
		case *token.Scalar:
			if x.IsKey() {
				keys := stack[len(stack)-1]
				k := x.ToString()
				if keys[k] {
					return true
				}
				keys[k] = true
			}
		}
	}
	return false
}

// position returns the 1-based line and byte column of offset in input.
func position(input string, offset int64) (line, col int64) {
	before := input[:offset]
	return int64(strings.Count(before, "\n")) + 1, offset - int64(strings.LastIndexByte(before, '\n'))
}

// sameJPVToken reports whether a, from the decoder, and b, from the scanner
// decoder, are the same token.  The scanner decoder did not mark the names
// in paths such as $.name as unescaped, though they are.
func sameJPVToken(a, b token.Token) bool {
	x, ok := a.(*token.Scalar)
	if !ok {
		return fmt.Sprintf("%T", a) == fmt.Sprintf("%T", b)
	}
	y, ok := b.(*token.Scalar)
	if !ok {
		return false
	}
	yFlags := y.TypeAndFlags
	if y.IsKey() && y.IsAlnum() {
		yFlags |= token.UnescapedMask
	}
	return x.TypeAndFlags == yFlags && bytes.Equal(x.Bytes, y.Bytes)
}

// FuzzJPVMatchesScanner checks that the JPV decoder decodes inputs as the
// one built on internal/scanner did, apart from the two things that it
// rejects on purpose: a line break between a path and its '=', and anything
// after a value on its line, such as another entry.  It also checks the
// position of each error, reading one byte at a time.
func FuzzJPVMatchesScanner(f *testing.F) {
	for _, seed := range []string{
		"$ = 1\n", "$.a = 1\n$.b = [2]\n", `$["a b"][0] = "x"` + "\n", "$.a[0] = 1\n$.a[1] = 2\n$.b.c = null\n",
		"$.a = {}\n$.b = []\n", "$[0 = 1\n", "$.a\n", "= 1\n", "$.a = \n", "$.a.b = 1\n$.a = 2\n", "$[1] = 1\n$[0] = 2\n",
		"$.a = 1 $.b = 2\n", "$.a\n= 1\n", "$.a =\n1\n", "$.a = [1,\n2]\n$.b = 3", "\n\n  $.a = 1  \r\n\t$.b = 2",
		`$["\u00e9"] = "\ud800"` + "\n", "$[\"\xff\"] = \"\xff\"\n", `$["a\"]"] = 1`, `$["]"] = 1`, "$.a = 01\n",
		"$[007] = 1\n", "$.a = 1\n$.a = 2\n", "$.a = 1 \n", "$.a = 1x\n", "$.a = \"x\n", "$.a = [\n\n", "$.é = 1\n",
		"$.a = 1\r\n$.b = 2\r\n", "$ = {\"a\":\n1}\n", `$ = "a" "b"`, "$.a =", "$.a = 1\n$", "$.a.b = 1\n$.a.c = 2",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		acc := token.NewAccumulatorStream()
		err := NewDecoder(iotest.OneByteReader(strings.NewReader(input))).Produce(acc)
		got := acc.GetTokens()
		acc = token.NewAccumulatorStream()
		wantErr := newScannerDecoder(strings.NewReader(input)).Produce(acc)
		want := acc.GetTokens()

		var serr *json.SyntaxError
		if errors.As(err, &serr) {
			if line, col := position(input, serr.Offset); serr.Line != line || serr.Column != col {
				t.Fatalf("input %q: error at L%d,C%d, want L%d,C%d: %v", input, serr.Line, serr.Column, line, col, err)
			}
		}
		switch {
		case err != nil && wantErr != nil:
			return
		case err != nil:
			// The two rejections on purpose must be what they say.
			restOfLine := func() string {
				rest, _, _ := strings.Cut(input[serr.Offset:], "\n")
				return strings.Trim(rest, " \t\r")
			}
			switch {
			case errors.Is(err, errLineEndsBeforeEquals):
				if restOfLine() != "" || !strings.Contains(input[serr.Offset:], "\n") {
					t.Fatalf("input %q: %v, but the line does not end there", input, err)
				}
				return
			case errors.Is(err, errMoreAfterValue):
				if restOfLine() == "" {
					t.Fatalf("input %q: %v, but the rest of the line is blank", input, err)
				}
				return
			}
			t.Fatalf("input %q\njsontext error: %v\nscanner accepts it: %v", input, err, want)
		case wantErr != nil:
			t.Fatalf("input %q\njsontext accepts it: %v\nscanner error: %v", input, got, wantErr)
		}
		if len(got) != len(want) {
			t.Fatalf("input %q\njsontext: %d tokens %v\nscanner:  %d tokens %v", input, len(got), got, len(want), want)
		}
		for i := range got {
			if !sameJPVToken(got[i], want[i]) {
				t.Fatalf("input %q: token %d differs\njsontext: %#v\nscanner:  %#v", input, i, got[i], want[i])
			}
		}
	})
}
