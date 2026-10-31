package json

import (
	"bytes"
	"strings"
	"testing"

	"github.com/arnodel/jsonstream/internal/format"
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
