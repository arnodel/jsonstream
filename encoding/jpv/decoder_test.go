package jpv

import (
	"io"
	"strings"
	"testing"

	"github.com/arnodel/jsonstream/token"
)

// TestDecoderSimpleValues tests decoding simple JPV values
func TestDecoderSimpleValues(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedCount int
	}{
		{
			name:          "single string value",
			input:         `$.name = "Alice"`,
			expectedCount: 4, // StartObject, key, value, EndObject
		},
		{
			name:          "single number value",
			input:         `$.age = 30`,
			expectedCount: 4,
		},
		{
			name:          "single boolean true",
			input:         `$.active = true`,
			expectedCount: 4,
		},
		{
			name:          "single boolean false",
			input:         `$.disabled = false`,
			expectedCount: 4,
		},
		{
			name:          "single null value",
			input:         `$.value = null`,
			expectedCount: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := decodeJPV(t, tt.input)
			if len(tokens) != tt.expectedCount {
				t.Errorf("expected %d tokens, got %d", tt.expectedCount, len(tokens))
			}
		})
	}
}

// TestDecoderArrays tests decoding JPV array representations
func TestDecoderArrays(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedCount int
	}{
		{
			name: "simple array",
			input: `$[0] = 1
$[1] = 2
$[2] = 3`,
			expectedCount: 5, // StartArray, 1, 2, 3, EndArray
		},
		{
			name: "nested array",
			input: `$[0][0] = 1
$[0][1] = 2
$[1][0] = 3`,
			expectedCount: 9, // StartArray, StartArray, 1, 2, EndArray, StartArray, 3, EndArray, EndArray
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := decodeJPV(t, tt.input)
			if len(tokens) != tt.expectedCount {
				t.Errorf("expected %d tokens, got %d", tt.expectedCount, len(tokens))
			}
		})
	}
}

// TestDecoderObjects tests decoding JPV object representations
func TestDecoderObjects(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedCount int
	}{
		{
			name: "simple object",
			input: `$.name = "Alice"
$.age = 30`,
			expectedCount: 6, // StartObject, "name", "Alice", "age", 30, EndObject
		},
		{
			name: "nested object",
			input: `$.user.name = "Bob"
$.user.email = "bob@example.com"`,
			expectedCount: 9, // StartObject, "user", StartObject, "name", "Bob", "email", "bob@...", EndObject, EndObject
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := decodeJPV(t, tt.input)
			if len(tokens) != tt.expectedCount {
				t.Errorf("expected %d tokens, got %d", tt.expectedCount, len(tokens))
			}
		})
	}
}

// TestDecoderBracketNotation tests bracket notation for keys
func TestDecoderBracketNotation(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "string key with brackets",
			input: `$["first-name"] = "Alice"`,
		},
		{
			name:  "numeric index",
			input: `$[0] = "first"`,
		},
		{
			name:  "mixed notation",
			input: `$.users[0].name = "Alice"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := decodeJPV(t, tt.input)
			if len(tokens) == 0 {
				t.Error("expected tokens, got none")
			}
		})
	}
}

// TestDecoderDotNotation tests dot notation for alphanumeric keys
func TestDecoderDotNotation(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "simple dot notation",
			input: `$.name = "Alice"`,
		},
		{
			name:  "nested dot notation",
			input: `$.user.profile.name = "Alice"`,
		},
		{
			name:  "alphanumeric key",
			input: `$.field_name_123 = "value"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := decodeJPV(t, tt.input)
			if len(tokens) == 0 {
				t.Error("expected tokens, got none")
			}
		})
	}
}

// TestDecoderComplexDocument tests a realistic JPV document
func TestDecoderComplexDocument(t *testing.T) {
	input := `$.name = "John Doe"
$.age = 30
$.email = "john@example.com"
$.address.street = "123 Main St"
$.address.city = "Springfield"
$.phones[0].type = "home"
$.phones[0].number = "555-1234"
$.phones[1].type = "work"
$.phones[1].number = "555-5678"
$.active = true`

	tokens := decodeJPV(t, input)
	if len(tokens) < 20 {
		t.Errorf("expected at least 20 tokens for complex document, got %d", len(tokens))
	}
}

// TestDecoderWhitespace tests handling of whitespace
func TestDecoderWhitespace(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "leading spaces",
			input: `   $.name = "Alice"`,
		},
		{
			name:  "spaces around equals",
			input: `$.name   =   "Alice"`,
		},
		{
			name:  "trailing spaces",
			input: `$.name = "Alice"   `,
		},
		{
			name: "empty lines",
			input: `$.name = "Alice"

$.age = 30`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := decodeJPV(t, tt.input)
			if len(tokens) == 0 {
				t.Error("expected tokens, got none")
			}
		})
	}
}

// TestDecoderEOF tests handling of EOF
func TestDecoderEOF(t *testing.T) {
	decoder := NewDecoder(strings.NewReader(""))
	out := make(chan token.Token, 100)

	err := decoder.Produce(out)
	close(out)

	if err != nil {
		t.Errorf("expected nil for empty input, got %v", err)
	}

	var tokens []token.Token
	for tok := range out {
		tokens = append(tokens, tok)
	}
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens, got %d", len(tokens))
	}
}

// TestDecoderUnclosedStructures tests that the decoder properly handles EOF when it encounters
// an unclosed structure. This test should expose the EOF handling bug.
func TestDecoderUnclosedStructures(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"unclosed path - missing bracket", `$.["key`},
		{"unclosed path - missing closing bracket", `$[0`},
		{"incomplete path - dot without key", `$.`},
		{"incomplete line - missing value", `$.name = `},
		{"incomplete line - missing equals", `$.name `},
		{"unclosed string in value", `$.name = "Alice`},
		{"unclosed array in value", `$.items = [1, 2, 3`},
		{"unclosed object in value", `$.user = {"name": "Alice"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewDecoder(strings.NewReader(tt.input))
			out := make(chan token.Token, 100)

			err := decoder.Produce(out)
			close(out)

			// Should get an error (io.EOF or syntax error) when the reader runs out of input while parsing
			if err == nil {
				t.Error("expected error for unclosed structure, got nil")
			}

			// Drain the channel
			for range out {
			}
		})
	}
}

// TestDecoderErrors tests various error conditions
func TestDecoderErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"missing dollar sign", `.name = "Alice"`},
		{"unclosed bracket in path", `$["name" = "Alice"`},
		{"empty bracket", `$[] = "value"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewDecoder(strings.NewReader(tt.input))
			out := make(chan token.Token, 100)

			err := decoder.Produce(out)
			close(out)

			if err == nil {
				t.Error("expected error, got nil")
			}

			// Drain the channel
			for range out {
			}
		})
	}
}

// Helper functions

// decodeJPV decodes a JPV string and returns all tokens
func decodeJPV(t *testing.T, input string) []token.Token {
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
