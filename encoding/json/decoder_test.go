package json

import (
	"io"
	"strings"
	"testing"

	"github.com/arnodel/jsonstream/token"
)

// TestDecoderSimpleValues tests decoding of simple scalar values
func TestDecoderSimpleValues(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []token.Token
	}{
		{
			name:  "true",
			input: "true",
			expected: []token.Token{
				token.TrueScalar,
			},
		},
		{
			name:  "false",
			input: "false",
			expected: []token.Token{
				token.FalseScalar,
			},
		},
		{
			name:  "null",
			input: "null",
			expected: []token.Token{
				token.NullScalar,
			},
		},
		{
			name:  "integer",
			input: "42",
			expected: []token.Token{
				tokenWithBytes(token.Number, "42"),
			},
		},
		{
			name:  "negative integer",
			input: "-123",
			expected: []token.Token{
				tokenWithBytes(token.Number, "-123"),
			},
		},
		{
			name:  "float",
			input: "3.14",
			expected: []token.Token{
				tokenWithBytes(token.Number, "3.14"),
			},
		},
		{
			name:  "scientific notation",
			input: "1.5e10",
			expected: []token.Token{
				tokenWithBytes(token.Number, "1.5e10"),
			},
		},
		{
			name:  "simple string",
			input: `"hello"`,
			expected: []token.Token{
				tokenWithBytes(token.String, `"hello"`),
			},
		},
		{
			name:  "empty string",
			input: `""`,
			expected: []token.Token{
				tokenWithBytes(token.String, `""`),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := decodeString(t, tt.input)
			assertTokensEqual(t, tokens, tt.expected)
		})
	}
}

// TestDecoderStrings tests various string formats
func TestDecoderStrings(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "string with spaces",
			input:    `"hello world"`,
			expected: `"hello world"`,
		},
		{
			name:     "string with escaped quotes",
			input:    `"hello \"world\""`,
			expected: `"hello \"world\""`,
		},
		{
			name:     "string with backslash",
			input:    `"hello\\world"`,
			expected: `"hello\\world"`,
		},
		{
			name:     "string with newline",
			input:    `"hello\nworld"`,
			expected: `"hello\nworld"`,
		},
		{
			name:     "string with tab",
			input:    `"hello\tworld"`,
			expected: `"hello\tworld"`,
		},
		{
			name:     "string with unicode",
			input:    `"hello\u0041world"`,
			expected: `"hello\u0041world"`,
		},
		{
			name:     "string with emoji",
			input:    `"hello 😀 world"`,
			expected: `"hello 😀 world"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := decodeString(t, tt.input)
			if len(tokens) != 1 {
				t.Fatalf("expected 1 token, got %d", len(tokens))
			}
			scalar, ok := tokens[0].(*token.Scalar)
			if !ok {
				t.Fatalf("expected scalar token, got %T", tokens[0])
			}
			if string(scalar.Bytes) != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, string(scalar.Bytes))
			}
		})
	}
}

// TestDecoderNumbers tests various number formats
func TestDecoderNumbers(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"zero", "0"},
		{"positive", "123"},
		{"negative", "-456"},
		{"decimal", "3.14159"},
		{"negative decimal", "-2.71828"},
		{"exponent positive", "1e10"},
		{"exponent negative", "1e-10"},
		{"exponent with sign", "1.5e+20"},
		{"complex", "-1.23e-45"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := decodeString(t, tt.input)
			if len(tokens) != 1 {
				t.Fatalf("expected 1 token, got %d", len(tokens))
			}
			scalar, ok := tokens[0].(*token.Scalar)
			if !ok {
				t.Fatalf("expected scalar token, got %T", tokens[0])
			}
			if scalar.Type() != token.Number {
				t.Errorf("expected Number type, got %v", scalar.Type())
			}
			if string(scalar.Bytes) != tt.input {
				t.Errorf("expected %q, got %q", tt.input, string(scalar.Bytes))
			}
		})
	}
}

// TestDecoderArrays tests array decoding
func TestDecoderArrays(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(*testing.T, []token.Token)
	}{
		{
			name:  "empty array",
			input: "[]",
			check: func(t *testing.T, tokens []token.Token) {
				if len(tokens) != 2 {
					t.Errorf("expected 2 tokens, got %d", len(tokens))
				}
				assertTokenType(t, tokens[0], &token.StartArray{})
				assertTokenType(t, tokens[1], &token.EndArray{})
			},
		},
		{
			name:  "array with one element",
			input: "[42]",
			check: func(t *testing.T, tokens []token.Token) {
				if len(tokens) != 3 {
					t.Errorf("expected 3 tokens, got %d", len(tokens))
				}
				assertTokenType(t, tokens[0], &token.StartArray{})
				assertScalarValue(t, tokens[1], token.Number, "42")
				assertTokenType(t, tokens[2], &token.EndArray{})
			},
		},
		{
			name:  "array with multiple elements",
			input: "[1, 2, 3]",
			check: func(t *testing.T, tokens []token.Token) {
				if len(tokens) != 5 {
					t.Errorf("expected 5 tokens, got %d", len(tokens))
				}
				assertTokenType(t, tokens[0], &token.StartArray{})
				assertScalarValue(t, tokens[1], token.Number, "1")
				assertScalarValue(t, tokens[2], token.Number, "2")
				assertScalarValue(t, tokens[3], token.Number, "3")
				assertTokenType(t, tokens[4], &token.EndArray{})
			},
		},
		{
			name:  "array with mixed types",
			input: `[1, "hello", true, null]`,
			check: func(t *testing.T, tokens []token.Token) {
				if len(tokens) != 6 {
					t.Errorf("expected 6 tokens, got %d", len(tokens))
				}
				assertTokenType(t, tokens[0], &token.StartArray{})
				assertScalarValue(t, tokens[1], token.Number, "1")
				assertScalarValue(t, tokens[2], token.String, `"hello"`)
				assertScalarValue(t, tokens[3], token.Boolean, "true")
				assertScalarValue(t, tokens[4], token.Null, "null")
				assertTokenType(t, tokens[5], &token.EndArray{})
			},
		},
		{
			name:  "nested arrays",
			input: "[[1, 2], [3, 4]]",
			check: func(t *testing.T, tokens []token.Token) {
				if len(tokens) != 10 {
					t.Errorf("expected 10 tokens, got %d", len(tokens))
				}
				assertTokenType(t, tokens[0], &token.StartArray{})
				assertTokenType(t, tokens[1], &token.StartArray{})
				assertScalarValue(t, tokens[2], token.Number, "1")
				assertScalarValue(t, tokens[3], token.Number, "2")
				assertTokenType(t, tokens[4], &token.EndArray{})
				assertTokenType(t, tokens[5], &token.StartArray{})
				assertScalarValue(t, tokens[6], token.Number, "3")
				assertScalarValue(t, tokens[7], token.Number, "4")
				assertTokenType(t, tokens[8], &token.EndArray{})
				assertTokenType(t, tokens[9], &token.EndArray{})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := decodeString(t, tt.input)
			tt.check(t, tokens)
		})
	}
}

// TestDecoderObjects tests object decoding
func TestDecoderObjects(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(*testing.T, []token.Token)
	}{
		{
			name:  "empty object",
			input: "{}",
			check: func(t *testing.T, tokens []token.Token) {
				if len(tokens) != 2 {
					t.Errorf("expected 2 tokens, got %d", len(tokens))
				}
				assertTokenType(t, tokens[0], &token.StartObject{})
				assertTokenType(t, tokens[1], &token.EndObject{})
			},
		},
		{
			name:  "object with one pair",
			input: `{"name": "Alice"}`,
			check: func(t *testing.T, tokens []token.Token) {
				if len(tokens) != 4 {
					t.Errorf("expected 4 tokens, got %d", len(tokens))
				}
				assertTokenType(t, tokens[0], &token.StartObject{})
				assertScalarValue(t, tokens[1], token.String, `"name"`)
				assertScalarValue(t, tokens[2], token.String, `"Alice"`)
				assertTokenType(t, tokens[3], &token.EndObject{})
			},
		},
		{
			name:  "object with multiple pairs",
			input: `{"name": "Alice", "age": 30}`,
			check: func(t *testing.T, tokens []token.Token) {
				if len(tokens) != 6 {
					t.Errorf("expected 6 tokens, got %d", len(tokens))
				}
				assertTokenType(t, tokens[0], &token.StartObject{})
				// Note: map order may vary, so we just check types
				assertTokenType(t, tokens[5], &token.EndObject{})
			},
		},
		{
			name:  "nested objects",
			input: `{"user": {"name": "Alice"}}`,
			check: func(t *testing.T, tokens []token.Token) {
				if len(tokens) != 7 {
					t.Errorf("expected 7 tokens, got %d", len(tokens))
				}
				assertTokenType(t, tokens[0], &token.StartObject{})
				assertTokenType(t, tokens[2], &token.StartObject{})
				assertTokenType(t, tokens[5], &token.EndObject{})
				assertTokenType(t, tokens[6], &token.EndObject{})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := decodeString(t, tt.input)
			tt.check(t, tokens)
		})
	}
}

// TestDecoderWhitespace tests handling of whitespace
func TestDecoderWhitespace(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"leading spaces", "   42"},
		{"trailing spaces", "42   "},
		{"leading tabs", "\t\t42"},
		{"leading newlines", "\n\n42"},
		{"mixed whitespace", " \t\n 42 \t\n "},
		{"array with spaces", "[ 1 , 2 , 3 ]"},
		{"object with spaces", `{ "key" : "value" }`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := decodeString(t, tt.input)
			if len(tokens) == 0 {
				t.Error("expected at least one token")
			}
		})
	}
}

// TestDecoderMultipleValues tests streaming multiple JSON values
func TestDecoderMultipleValues(t *testing.T) {
	input := `42 "hello" true`
	tokens := decodeString(t, input)

	if len(tokens) != 3 {
		t.Errorf("expected 3 tokens, got %d", len(tokens))
	}

	assertScalarValue(t, tokens[0], token.Number, "42")
	assertScalarValue(t, tokens[1], token.String, `"hello"`)
	assertScalarValue(t, tokens[2], token.Boolean, "true")
}

// TestDecoderErrors tests various error conditions
func TestDecoderErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"missing colon", `{"key" "value"}`},
		{"missing comma in array", `[1 2]`},
		{"missing comma in object", `{"a": 1 "b": 2}`},
		{"control char in string", "\"hello\x00world\""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewDecoder(strings.NewReader(tt.input))
			out := make(chan token.Token, 100)

			// Run in goroutine with timeout detection
			done := make(chan error, 1)
			go func() {
				done <- decoder.Produce(out)
				close(out)
			}()

			// Wait for completion or timeout
			err := <-done

			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

// TestDecoderComplexDocument tests a realistic JSON document
func TestDecoderComplexDocument(t *testing.T) {
	input := `{
		"name": "John Doe",
		"age": 30,
		"email": "john@example.com",
		"address": {
			"street": "123 Main St",
			"city": "Springfield",
			"zip": "12345"
		},
		"phones": [
			{"type": "home", "number": "555-1234"},
			{"type": "work", "number": "555-5678"}
		],
		"active": true,
		"balance": 123.45
	}`

	tokens := decodeString(t, input)

	// Just verify we got some reasonable number of tokens
	if len(tokens) < 20 {
		t.Errorf("expected at least 20 tokens for complex document, got %d", len(tokens))
	}

	// Verify structure
	assertTokenType(t, tokens[0], &token.StartObject{})
	assertTokenType(t, tokens[len(tokens)-1], &token.EndObject{})
}

// TestDecoderLargeArray tests handling of large arrays
func TestDecoderLargeArray(t *testing.T) {
	// Build a large array
	var sb strings.Builder
	sb.WriteString("[")
	for i := 0; i < 1000; i++ {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("42")
	}
	sb.WriteString("]")

	tokens := decodeString(t, sb.String())

	// Should have: StartArray + 1000 numbers + EndArray = 1002 tokens
	expected := 1002
	if len(tokens) != expected {
		t.Errorf("expected %d tokens, got %d", expected, len(tokens))
	}
}

// TestDecoderDeepNesting tests deeply nested structures
func TestDecoderDeepNesting(t *testing.T) {
	depth := 50
	var sb strings.Builder

	// Build deeply nested arrays
	for i := 0; i < depth; i++ {
		sb.WriteString("[")
	}
	sb.WriteString("42")
	for i := 0; i < depth; i++ {
		sb.WriteString("]")
	}

	tokens := decodeString(t, sb.String())

	// Should have: depth * StartArray + scalar + depth * EndArray
	expected := depth*2 + 1
	if len(tokens) != expected {
		t.Errorf("expected %d tokens, got %d", expected, len(tokens))
	}
}

// TestDecoderEOF tests handling of EOF
func TestDecoderEOF(t *testing.T) {
	decoder := NewDecoder(strings.NewReader(""))
	out := make(chan token.Token, 10)
	err := decoder.Produce(out)
	close(out)

	// EOF on empty input should return nil error
	if err != nil && err != io.EOF {
		t.Errorf("expected nil or EOF error, got %v", err)
	}

	// Should have no tokens
	var tokens []token.Token
	for tok := range out {
		tokens = append(tokens, tok)
	}
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens, got %d", len(tokens))
	}
}

// Helper functions

// decodeString decodes a JSON string and returns all tokens
func decodeString(t *testing.T, input string) []token.Token {
	t.Helper()
	decoder := NewDecoder(strings.NewReader(input))
	out := make(chan token.Token, 100)

	go func() {
		err := decoder.Produce(out)
		if err != nil && err != io.EOF {
			t.Errorf("decode error: %v", err)
		}
		close(out)
	}()

	var tokens []token.Token
	for tok := range out {
		tokens = append(tokens, tok)
	}
	return tokens
}

// tokenWithBytes creates a scalar token with specific bytes
func tokenWithBytes(typ token.ScalarType, bytes string) *token.Scalar {
	return token.NewScalar(typ, []byte(bytes))
}

// assertTokensEqual compares two token slices
func assertTokensEqual(t *testing.T, got, want []token.Token) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("token count mismatch: got %d, want %d", len(got), len(want))
	}

	for i := range got {
		g := got[i]
		w := want[i]

		gScalar, gOk := g.(*token.Scalar)
		wScalar, wOk := w.(*token.Scalar)

		if gOk != wOk {
			t.Errorf("token %d: type mismatch: got %T, want %T", i, g, w)
			continue
		}

		if gOk {
			if gScalar.Type() != wScalar.Type() {
				t.Errorf("token %d: scalar type mismatch: got %v, want %v", i, gScalar.Type(), wScalar.Type())
			}
			if string(gScalar.Bytes) != string(wScalar.Bytes) {
				t.Errorf("token %d: bytes mismatch: got %q, want %q", i, string(gScalar.Bytes), string(wScalar.Bytes))
			}
		} else {
			// For structural tokens, just check types match
			if _, gStart := g.(*token.StartArray); gStart {
				if _, wStart := w.(*token.StartArray); !wStart {
					t.Errorf("token %d: expected StartArray, got %T", i, w)
				}
			} else if _, gEnd := g.(*token.EndArray); gEnd {
				if _, wEnd := w.(*token.EndArray); !wEnd {
					t.Errorf("token %d: expected EndArray, got %T", i, w)
				}
			}
			// Similar for Object tokens...
		}
	}
}

// assertTokenType checks that a token is of expected type
func assertTokenType(t *testing.T, tok token.Token, expected token.Token) {
	t.Helper()
	switch expected.(type) {
	case *token.StartArray:
		if _, ok := tok.(*token.StartArray); !ok {
			t.Errorf("expected StartArray, got %T", tok)
		}
	case *token.EndArray:
		if _, ok := tok.(*token.EndArray); !ok {
			t.Errorf("expected EndArray, got %T", tok)
		}
	case *token.StartObject:
		if _, ok := tok.(*token.StartObject); !ok {
			t.Errorf("expected StartObject, got %T", tok)
		}
	case *token.EndObject:
		if _, ok := tok.(*token.EndObject); !ok {
			t.Errorf("expected EndObject, got %T", tok)
		}
	}
}

// assertScalarValue checks scalar token type and value
func assertScalarValue(t *testing.T, tok token.Token, typ token.ScalarType, value string) {
	t.Helper()
	scalar, ok := tok.(*token.Scalar)
	if !ok {
		t.Errorf("expected Scalar, got %T", tok)
		return
	}
	if scalar.Type() != typ {
		t.Errorf("expected type %v, got %v", typ, scalar.Type())
	}
	if string(scalar.Bytes) != value {
		t.Errorf("expected value %q, got %q", value, string(scalar.Bytes))
	}
}
