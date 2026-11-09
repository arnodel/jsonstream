package json

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/arnodel/jsonstream/format"
	"github.com/arnodel/jsonstream/internal/scanner"
	"github.com/arnodel/jsonstream/token"
)

// decodeAll decodes the input into a slice of tokens.
func decodeAll(input string) ([]token.Token, error) {
	acc := token.NewAccumulatorStream()
	err := NewDecoder(strings.NewReader(input)).Produce(acc)
	return acc.GetTokens(), err
}

// encodeCompact encodes tokens as compact JSON, one value per line.
func encodeCompact(toks []token.Token) string {
	var buf bytes.Buffer
	enc := &Encoder{Printer: &format.DefaultPrinter{Writer: &buf, IndentSize: -1}}
	if err := enc.Consume(token.NewSliceReadStream(toks)); err != nil {
		panic(err)
	}
	return buf.String()
}

// checkWellFormed checks that the tokens form a sequence of valid JSON
// values: balanced containers, keys only in objects and followed by values.
func checkWellFormed(t *testing.T, toks []token.Token) {
	t.Helper()
	var stack []byte // '[' or '{' for each open container
	top := func() byte {
		if len(stack) == 0 {
			return 0
		}
		return stack[len(stack)-1]
	}
	expectKey := false // true when the next token must be an object key
	for i, tok := range toks {
		switch x := tok.(type) {
		case *token.StartArray:
			if expectKey {
				t.Fatalf("token %d: array where a key was expected", i)
			}
			stack = append(stack, '[')
			expectKey = false
			continue
		case *token.StartObject:
			if expectKey {
				t.Fatalf("token %d: object where a key was expected", i)
			}
			stack = append(stack, '{')
			expectKey = true
			continue
		case *token.EndArray:
			if top() != '[' {
				t.Fatalf("token %d: unbalanced ]", i)
			}
			stack = stack[:len(stack)-1]
		case *token.EndObject:
			if top() != '{' || !expectKey {
				t.Fatalf("token %d: unbalanced or misplaced }", i)
			}
			stack = stack[:len(stack)-1]
		case *token.Scalar:
			if expectKey {
				if !x.IsKey() || x.Type() != token.String {
					t.Fatalf("token %d: expected a string key, got %s", i, x)
				}
				expectKey = false
				continue
			}
			if x.IsKey() {
				t.Fatalf("token %d: key where a value was expected", i)
			}
		default:
			t.Fatalf("token %d: unexpected token %T", i, tok)
		}
		// A value (scalar or closed container) has just ended: inside an
		// object the next token is a key.
		expectKey = top() == '{'
	}
	if len(stack) != 0 {
		t.Fatalf("unclosed containers at end of stream: %q", stack)
	}
}

func tokensEqual(a, b []token.Token) bool {
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

// FuzzDecoder checks that the decoder never panics, and that whatever it
// accepts is well-formed and survives a round trip through the encoder.
func FuzzDecoder(f *testing.F) {
	for _, seed := range []string{
		`{}`, `[]`, `null`, `"a"`, `-0.5e-3`, `[1, [2, [3]]]`, `{"a": {"b": [true, false, null]}}`,
		`{"a": 1} {"b": 2}` + "\n" + `[3]`, `"é😀\n\\\""`, `{"": ""}`, `[1,]`, `{"a" 1}`,
		`123456789012345678901234567890`, `1e`, `"unterminated`, `[`, `tru`, "\x00", ` `, `"\0"`, `{"0": 0, "\0": 0}`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		toks, err := decodeAll(input)
		if err != nil {
			// Invalid input: what was decoded before the error still has to be
			// a prefix of a well-formed stream, which the round trip below
			// does not check, so stop here.
			return
		}
		checkWellFormed(t, toks)
		again, err := decodeAll(encodeCompact(toks))
		if err != nil {
			t.Fatalf("re-decoding the encoder's output failed: %v\ninput: %q\noutput: %q", err, input, encodeCompact(toks))
		}
		if !tokensEqual(toks, again) {
			t.Fatalf("round trip changed the tokens\ninput: %q\noutput: %q", input, encodeCompact(toks))
		}
	})
}

// decodeOneByteAtATime decodes the input read one byte at a time.
func decodeOneByteAtATime(input string) ([]token.Token, error) {
	acc := token.NewAccumulatorStream()
	err := NewDecoder(iotest.OneByteReader(strings.NewReader(input))).Produce(acc)
	return acc.GetTokens(), err
}

// position returns the 1-based line and byte column of offset in input.
func position(input string, offset int64) (line, col int64) {
	before := input[:offset]
	return int64(strings.Count(before, "\n")) + 1, offset - int64(strings.LastIndexByte(before, '\n'))
}

// decodeWithScanner decodes the input with the decoder that the jpv format
// uses, which parses JSON with internal/scanner rather than jsontext.
func decodeWithScanner(input string) ([]token.Token, error) {
	acc := token.NewAccumulatorStream()
	err := NewDecoderFromScanner(scanner.NewScanner(strings.NewReader(input))).Produce(acc)
	return acc.GetTokens(), err
}

// sameToken reports whether a and b are the same token, scalars with the same
// type, flags and bytes.
func sameToken(a, b token.Token) bool {
	x, ok := a.(*token.Scalar)
	if !ok {
		return fmt.Sprintf("%T", a) == fmt.Sprintf("%T", b)
	}
	y, ok := b.(*token.Scalar)
	return ok && x.TypeAndFlags == y.TypeAndFlags && bytes.Equal(x.Bytes, y.Bytes)
}

// FuzzDecoderMatchesScanner checks that the jsontext decoder and the scanner
// decoder accept the same inputs and decode them to the same tokens, and that
// the jsontext decoder reports errors at the right line and column.  It
// reads one byte at a time, as the input from a pipe can arrive in any
// pieces.
func FuzzDecoderMatchesScanner(f *testing.F) {
	for _, seed := range []string{
		`{}`, `[]`, `null`, `"a"`, `-0.5e-3`, `[1, [2, [3]]]`, `{"a": {"b": [true, false, null]}}`,
		`{"a": 1} {"b": 2}` + "\n" + `[3]`, `"é😀\n\\\""`, `{"": ""}`, `[1,]`, `{"a" 1}`,
		`123456789012345678901234567890`, `1e`, `"unterminated`, `[`, `tru`, "\x00", ` `, `"\0"`, `{"0": 0, "\0": 0}`,
		`01`, `1 2`, `12`, `"a""b"`, `{}{}`, `[1]2`, `truefalse`, `1-`, `-`, `1.`, `.5`, `1e+`, `+1`,
		`"\ud800"`, `"\udc00\ud800"`, `"é"`, "\"\xff\"", "\"\xc3\"", "{\"\xff\": 1}", "\xff",
		"\ufeff{}", "\t\r\n 1 \t\r\n", `{"a": 1, "a": 2}`, `{"x1": 1, "1x": 2, "_": 3, "a b": 4}`,
		`[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[1]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]`, `1/`, `/* */ 1`, `NaN`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		got, err := decodeOneByteAtATime(input)
		want, wantErr := decodeWithScanner(input)
		if (err == nil) != (wantErr == nil) {
			t.Fatalf("input %q\njsontext error: %v\nscanner error:  %v", input, err, wantErr)
		}
		if err != nil {
			var serr *SyntaxError
			if !errors.As(err, &serr) {
				t.Fatalf("input %q: not a *SyntaxError: %v", input, err)
			}
			if line, col := position(input, serr.Offset); serr.Line != line || serr.Column != col {
				t.Fatalf("input %q: error at L%d,C%d, want L%d,C%d: %v", input, serr.Line, serr.Column, line, col, err)
			}
			return
		}
		if len(got) != len(want) {
			t.Fatalf("input %q\njsontext: %d tokens %v\nscanner:  %d tokens %v", input, len(got), got, len(want), want)
		}
		for i := range got {
			if !sameToken(got[i], want[i]) {
				t.Fatalf("input %q: token %d differs\njsontext: %#v\nscanner:  %#v", input, i, got[i], want[i])
			}
		}
	})
}
