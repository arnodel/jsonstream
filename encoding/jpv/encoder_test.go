package jpv

import (
	"strings"
	"testing"

	"github.com/arnodel/jsonstream/internal/format"
	"github.com/arnodel/jsonstream/token"
)

// TestEncoderSimpleValues tests encoding simple values to JPV
func TestEncoderSimpleValues(t *testing.T) {
	tests := []struct {
		name     string
		tokens   []token.Token
		expected string
	}{
		{
			name:     "string value",
			tokens:   []token.Token{token.StringScalar("hello")},
			expected: `$ = "hello"`,
		},
		{
			name:     "number value",
			tokens:   []token.Token{token.Int64Scalar(42)},
			expected: `$ = 42`,
		},
		{
			name:     "boolean true",
			tokens:   []token.Token{token.TrueScalar},
			expected: `$ = true`,
		},
		{
			name:     "boolean false",
			tokens:   []token.Token{token.FalseScalar},
			expected: `$ = false`,
		},
		{
			name:     "null value",
			tokens:   []token.Token{token.NullScalar},
			expected: `$ = null`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := encodeJPV(t, tt.tokens)
			if strings.TrimSpace(output) != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, strings.TrimSpace(output))
			}
		})
	}
}

// TestEncoderArrays tests encoding arrays to JPV format
func TestEncoderArrays(t *testing.T) {
	tests := []struct {
		name     string
		tokens   []token.Token
		expected []string
	}{
		{
			name: "simple array",
			tokens: []token.Token{
				&token.StartArray{},
				token.Int64Scalar(1),
				token.Int64Scalar(2),
				token.Int64Scalar(3),
				&token.EndArray{},
			},
			expected: []string{
				"$[0] = 1",
				"$[1] = 2",
				"$[2] = 3",
			},
		},
		{
			name: "empty array",
			tokens: []token.Token{
				&token.StartArray{},
				&token.EndArray{},
			},
			expected: []string{
				"$ = []",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := encodeJPV(t, tt.tokens)
			lines := strings.Split(strings.TrimSpace(output), "\n")
			if len(lines) != len(tt.expected) {
				t.Errorf("expected %d lines, got %d", len(tt.expected), len(lines))
			}
			for i, expected := range tt.expected {
				if i >= len(lines) {
					break
				}
				if lines[i] != expected {
					t.Errorf("line %d: expected %q, got %q", i, expected, lines[i])
				}
			}
		})
	}
}

// TestEncoderObjects tests encoding objects to JPV format
func TestEncoderObjects(t *testing.T) {
	tests := []struct {
		name     string
		tokens   []token.Token
		expected []string
	}{
		{
			name: "simple object",
			tokens: []token.Token{
				&token.StartObject{},
				token.StringScalar("name"),
				token.StringScalar("Alice"),
				&token.EndObject{},
			},
			expected: []string{
				`$["name"] = "Alice"`,
			},
		},
		{
			name: "object with multiple fields",
			tokens: []token.Token{
				&token.StartObject{},
				token.StringScalar("name"),
				token.StringScalar("Alice"),
				token.StringScalar("age"),
				token.Int64Scalar(30),
				&token.EndObject{},
			},
			expected: []string{
				`$["name"] = "Alice"`,
				`$["age"] = 30`,
			},
		},
		{
			name: "empty object",
			tokens: []token.Token{
				&token.StartObject{},
				&token.EndObject{},
			},
			expected: []string{
				`$ = {}`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := encodeJPV(t, tt.tokens)
			lines := strings.Split(strings.TrimSpace(output), "\n")
			if len(lines) != len(tt.expected) {
				t.Errorf("expected %d lines, got %d:\n%s", len(tt.expected), len(lines), output)
			}
			for i, expected := range tt.expected {
				if i >= len(lines) {
					break
				}
				if lines[i] != expected {
					t.Errorf("line %d: expected %q, got %q", i, expected, lines[i])
				}
			}
		})
	}
}

// TestEncoderNestedStructures tests encoding nested structures
func TestEncoderNestedStructures(t *testing.T) {
	tests := []struct {
		name     string
		tokens   []token.Token
		expected []string
	}{
		{
			name: "nested object",
			tokens: []token.Token{
				&token.StartObject{},
				token.StringScalar("user"),
				&token.StartObject{},
				token.StringScalar("name"),
				token.StringScalar("Bob"),
				&token.EndObject{},
				&token.EndObject{},
			},
			expected: []string{
				`$["user"]["name"] = "Bob"`,
			},
		},
		{
			name: "nested array",
			tokens: []token.Token{
				&token.StartArray{},
				&token.StartArray{},
				token.Int64Scalar(1),
				token.Int64Scalar(2),
				&token.EndArray{},
				&token.EndArray{},
			},
			expected: []string{
				`$[0][0] = 1`,
				`$[0][1] = 2`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := encodeJPV(t, tt.tokens)
			lines := strings.Split(strings.TrimSpace(output), "\n")
			if len(lines) != len(tt.expected) {
				t.Errorf("expected %d lines, got %d:\n%s", len(tt.expected), len(lines), output)
			}
			for i, expected := range tt.expected {
				if i >= len(lines) {
					break
				}
				if lines[i] != expected {
					t.Errorf("line %d: expected %q, got %q", i, expected, lines[i])
				}
			}
		})
	}
}

// TestRoundtrip tests encoding then decoding
func TestRoundtrip(t *testing.T) {
	tests := []struct {
		name   string
		tokens []token.Token
	}{
		{
			name:   "simple value",
			tokens: []token.Token{token.Int64Scalar(42)},
		},
		{
			name: "array",
			tokens: []token.Token{
				&token.StartArray{},
				token.Int64Scalar(1),
				token.Int64Scalar(2),
				&token.EndArray{},
			},
		},
		{
			name: "object",
			tokens: []token.Token{
				&token.StartObject{},
				token.StringScalar("name"),
				token.StringScalar("Alice"),
				&token.EndObject{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode
			jpvOutput := encodeJPV(t, tt.tokens)

			// Decode
			decodedTokens := decodeJPV(t, jpvOutput)

			// Should have same number of tokens
			if len(decodedTokens) != len(tt.tokens) {
				t.Errorf("token count mismatch: input=%d, output=%d", len(tt.tokens), len(decodedTokens))
			}
		})
	}
}

// Helper functions

// encodeJPV encodes tokens to JPV format and returns the output string
func encodeJPV(t *testing.T, tokens []token.Token) string {
	t.Helper()
	var buf strings.Builder
	encoder := &Encoder{
		Printer: &format.DefaultPrinter{Writer: &buf},
	}

	tokenChan := make(chan token.Token, len(tokens))
	for _, tok := range tokens {
		tokenChan <- tok
	}
	close(tokenChan)

	err := encoder.Consume(tokenChan)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}

	return buf.String()
}
