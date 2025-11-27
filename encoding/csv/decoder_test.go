package csv

import (
	"io"
	"strings"
	"testing"

	"github.com/arnodel/jsonstream/token"
)

// TestDecoderSimpleArrays tests basic CSV decoding to arrays
func TestDecoderSimpleArrays(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedCount int
	}{
		{
			name:          "single row",
			input:         "1,2,3",
			expectedCount: 5, // StartArray, 1, 2, 3, EndArray
		},
		{
			name: "multiple rows",
			input: `1,2,3
4,5,6`,
			expectedCount: 10, // 2 arrays with 3 elements each
		},
		{
			name:          "single value",
			input:         "hello",
			expectedCount: 3, // StartArray, "hello", EndArray
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewDecoder(strings.NewReader(tt.input))
			tokens := decodeCSV(t, decoder)
			if len(tokens) != tt.expectedCount {
				t.Errorf("expected %d tokens, got %d", tt.expectedCount, len(tokens))
			}
		})
	}
}

// TestDecoderSimpleObjects tests CSV decoding to objects without headers
func TestDecoderSimpleObjects(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedCount int
	}{
		{
			name:          "single row",
			input:         "1,2,3",
			expectedCount: 8, // StartObject, field_1, 1, field_2, 2, field_3, 3, EndObject
		},
		{
			name: "multiple rows",
			input: `1,2
3,4`,
			expectedCount: 12, // 2 objects with 2 fields each (6 tokens per object)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewDecoder(strings.NewReader(tt.input))
			decoder.RecordsProduceObjects = true
			tokens := decodeCSV(t, decoder)
			if len(tokens) != tt.expectedCount {
				t.Errorf("expected %d tokens, got %d", tt.expectedCount, len(tokens))
			}
		})
	}
}

// TestDecoderWithHeader tests CSV decoding with headers
func TestDecoderWithHeader(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name: "simple header",
			input: `name,age,active
Alice,30,true
Bob,25,false`,
		},
		{
			name: "alphanumeric headers",
			input: `first_name,last_name,user_id
Alice,Smith,123
Bob,Jones,456`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewDecoder(strings.NewReader(tt.input))
			decoder.HasHeader = true
			decoder.RecordsProduceObjects = true
			tokens := decodeCSV(t, decoder)

			// Should have 2 data rows (header is consumed)
			// Each row: StartObject + 3 fields (key + value pairs) + EndObject = 8 tokens
			expectedCount := 16
			if len(tokens) != expectedCount {
				t.Errorf("expected %d tokens, got %d", expectedCount, len(tokens))
			}
		})
	}
}

// TestDecoderFieldTypes tests detection of different field types
func TestDecoderFieldTypes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []token.ScalarType
	}{
		{
			name:     "string field",
			input:    "hello",
			expected: []token.ScalarType{token.String},
		},
		{
			name:     "integer field",
			input:    "42",
			expected: []token.ScalarType{token.Number},
		},
		{
			name:     "float field",
			input:    "3.14",
			expected: []token.ScalarType{token.Number},
		},
		{
			name:     "boolean true",
			input:    "true",
			expected: []token.ScalarType{token.Boolean},
		},
		{
			name:     "boolean false",
			input:    "false",
			expected: []token.ScalarType{token.Boolean},
		},
		{
			name:     "empty field (null)",
			input:    ",",  // Empty field between commas
			expected: []token.ScalarType{token.Null},
		},
		{
			name:     "negative number",
			input:    "-123",
			expected: []token.ScalarType{token.Number},
		},
		{
			name:     "scientific notation",
			input:    "1.5e10",
			expected: []token.ScalarType{token.Number},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewDecoder(strings.NewReader(tt.input))
			tokens := decodeCSV(t, decoder)

			// tokens should be: StartArray, value(s), EndArray
			// But "," produces two fields, so 4 tokens
			if len(tokens) < 3 {
				t.Fatalf("expected at least 3 tokens, got %d", len(tokens))
			}

			// Find the first scalar (after StartArray)
			var scalar *token.Scalar
			for i := 1; i < len(tokens)-1; i++ {
				if s, ok := tokens[i].(*token.Scalar); ok {
					scalar = s
					break
				}
			}

			if scalar == nil {
				t.Fatal("expected to find a scalar token")
			}

			if scalar.Type() != tt.expected[0] {
				t.Errorf("expected type %v, got %v", tt.expected[0], scalar.Type())
			}
		})
	}
}

// TestDecoderMixedTypes tests CSV with mixed field types
func TestDecoderMixedTypes(t *testing.T) {
	input := `Alice,30,true
Bob,25,false
Charlie,,true`

	decoder := NewDecoder(strings.NewReader(input))
	tokens := decodeCSV(t, decoder)

	// Should have 3 arrays, each with StartArray + 3 values + EndArray
	expectedCount := 15
	if len(tokens) != expectedCount {
		t.Errorf("expected %d tokens, got %d", expectedCount, len(tokens))
	}

	// Check the third row has a null for the age field
	// Third row starts at index 10: StartArray, "Charlie", null, true, EndArray
	if scalar, ok := tokens[12].(*token.Scalar); ok {
		if scalar.Type() != token.Null {
			t.Errorf("expected null for empty field, got %v", scalar.Type())
		}
	}
}

// TestDecoderEscapeSequences tests handling of escape sequences
func TestDecoderEscapeSequences(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "double quotes",
			input: `"hello ""world"""`,
		},
		{
			name:  "newline in field",
			input: "\"hello\nworld\"",
		},
		{
			name:  "backslash",
			input: `hello\world`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewDecoder(strings.NewReader(tt.input))
			tokens := decodeCSV(t, decoder)

			// Should decode without error
			if len(tokens) == 0 {
				t.Error("expected tokens, got none")
			}

			// First value should be a string
			if len(tokens) >= 2 {
				if scalar, ok := tokens[1].(*token.Scalar); ok {
					if scalar.Type() != token.String {
						t.Errorf("expected string type, got %v", scalar.Type())
					}
				}
			}
		})
	}
}

// TestDecoderFieldNames tests field name generation
func TestDecoderFieldNames(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		hasHeader       bool
		expectedField   string
		expectedIsAlnum bool
	}{
		{
			name:            "auto-generated field names",
			input:           "1,2,3",
			hasHeader:       false,
			expectedField:   "field_1",
			expectedIsAlnum: true,
		},
		{
			name:            "alphanumeric header",
			input:           "user_id\n123",
			hasHeader:       true,
			expectedField:   "user_id",
			expectedIsAlnum: true,
		},
		{
			name:            "non-alphanumeric header",
			input:           "user-id\n123",
			hasHeader:       true,
			expectedField:   "user-id",
			expectedIsAlnum: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewDecoder(strings.NewReader(tt.input))
			decoder.HasHeader = tt.hasHeader
			decoder.RecordsProduceObjects = true
			tokens := decodeCSV(t, decoder)

			// Find the first key token (after StartObject)
			var keyToken *token.Scalar
			for _, tok := range tokens {
				if scalar, ok := tok.(*token.Scalar); ok && scalar.IsKey() {
					keyToken = scalar
					break
				}
			}

			if keyToken == nil {
				t.Fatal("expected to find a key token")
			}

			if keyToken.IsAlnum() != tt.expectedIsAlnum {
				t.Errorf("expected IsAlnum=%v, got %v", tt.expectedIsAlnum, keyToken.IsAlnum())
			}
		})
	}
}

// TestDecoderEmptyInput tests handling of empty input
func TestDecoderEmptyInput(t *testing.T) {
	decoder := NewDecoder(strings.NewReader(""))
	tokens := decodeCSV(t, decoder)

	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens for empty input, got %d", len(tokens))
	}
}

// TestDecoderSingleColumn tests CSV with a single column
func TestDecoderSingleColumn(t *testing.T) {
	input := `name
Alice
Bob
Charlie`

	decoder := NewDecoder(strings.NewReader(input))
	decoder.HasHeader = true
	decoder.RecordsProduceObjects = true
	tokens := decodeCSV(t, decoder)

	// Should have 3 data rows (header consumed)
	// Each row: StartObject + key + value + EndObject = 4 tokens
	expectedCount := 12
	if len(tokens) != expectedCount {
		t.Errorf("expected %d tokens, got %d", expectedCount, len(tokens))
	}
}

// TestDecoderNumberEdgeCases tests edge cases in number detection
func TestDecoderNumberEdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		expectType token.ScalarType
	}{
		{
			name:       "leading zero",
			input:      "0123",
			expectType: token.Number,
		},
		{
			name:       "decimal point only",
			input:      ".",
			expectType: token.String, // Invalid number, should be string
		},
		{
			name:       "letters and numbers",
			input:      "123abc",
			expectType: token.String,
		},
		{
			name:       "number-like but invalid",
			input:      "1.2.3",
			expectType: token.Number, // Parses as 1.2, rest is ignored by CSV field parser
		},
		{
			name:       "zero",
			input:      "0",
			expectType: token.Number,
		},
		{
			name:       "float with exponent",
			input:      "1.5e-10",
			expectType: token.Number,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewDecoder(strings.NewReader(tt.input))
			tokens := decodeCSV(t, decoder)

			// tokens should be: StartArray, value, EndArray
			if len(tokens) != 3 {
				t.Fatalf("expected 3 tokens, got %d", len(tokens))
			}

			scalar, ok := tokens[1].(*token.Scalar)
			if !ok {
				t.Fatalf("expected token to be Scalar, got %T", tokens[1])
			}

			if scalar.Type() != tt.expectType {
				t.Errorf("expected type %v, got %v (value: %s)", tt.expectType, scalar.Type(), string(scalar.Bytes))
			}
		})
	}
}

// TestDecoderMalformedCSV tests handling of malformed CSV
func TestDecoderMalformedCSV(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "unclosed quote",
			input: `"hello`,
		},
		{
			name: "mismatched columns",
			input: `1,2,3
4,5`, // Fewer columns in second row (should still parse)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewDecoder(strings.NewReader(tt.input))
			out := make(chan token.Token, 100)

			err := decoder.Produce(out)
			close(out)

			// For unclosed quote, expect an error
			// For mismatched columns, Go's csv.Reader is permissive by default
			if tt.name == "unclosed quote" && err == nil {
				t.Error("expected error for unclosed quote, got nil")
			}

			// Drain the channel
			for range out {
			}
		})
	}
}

// TestDecoderSetFieldNames tests the SetFieldNames method
func TestDecoderSetFieldNames(t *testing.T) {
	decoder := NewDecoder(strings.NewReader("1,2,3"))
	decoder.RecordsProduceObjects = true
	decoder.SetFieldNames([]string{"foo", "bar", "baz"})

	tokens := decodeCSV(t, decoder)

	// Find all key tokens
	var keys []string
	for _, tok := range tokens {
		if scalar, ok := tok.(*token.Scalar); ok && scalar.IsKey() {
			// Extract the key name (remove quotes)
			keyBytes := scalar.Bytes
			if len(keyBytes) >= 2 && keyBytes[0] == '"' && keyBytes[len(keyBytes)-1] == '"' {
				keys = append(keys, string(keyBytes[1:len(keyBytes)-1]))
			}
		}
	}

	expectedKeys := []string{"foo", "bar", "baz"}
	if len(keys) != len(expectedKeys) {
		t.Fatalf("expected %d keys, got %d", len(expectedKeys), len(keys))
	}

	for i, expected := range expectedKeys {
		if keys[i] != expected {
			t.Errorf("key %d: expected %q, got %q", i, expected, keys[i])
		}
	}
}

// TestDecoderExtraColumns tests handling when more columns than field names
func TestDecoderExtraColumns(t *testing.T) {
	decoder := NewDecoder(strings.NewReader("1,2,3,4,5"))
	decoder.RecordsProduceObjects = true
	decoder.SetFieldNames([]string{"a", "b"}) // Only 2 field names, but 5 columns

	tokens := decodeCSV(t, decoder)

	// Should generate field names for missing columns
	// StartObject + 5 key-value pairs + EndObject = 12 tokens
	expectedCount := 12
	if len(tokens) != expectedCount {
		t.Errorf("expected %d tokens, got %d", expectedCount, len(tokens))
	}
}

// TestDecoderComplexDocument tests a realistic CSV document
func TestDecoderComplexDocument(t *testing.T) {
	input := `name,age,city,active
Alice,30,NYC,true
Bob,25,LA,false
Charlie,35,Chicago,true
Diana,,Seattle,false`

	decoder := NewDecoder(strings.NewReader(input))
	decoder.HasHeader = true
	decoder.RecordsProduceObjects = true

	tokens := decodeCSV(t, decoder)

	// 4 data rows, each with 4 fields
	// Each row: StartObject + 4 key-value pairs (8 tokens) + EndObject = 10 tokens
	expectedCount := 40
	if len(tokens) != expectedCount {
		t.Errorf("expected %d tokens, got %d", expectedCount, len(tokens))
	}

	// Verify we have proper object structure
	objectCount := 0
	for _, tok := range tokens {
		if _, ok := tok.(*token.StartObject); ok {
			objectCount++
		}
	}

	if objectCount != 4 {
		t.Errorf("expected 4 objects, got %d", objectCount)
	}
}

// Helper functions

// decodeCSV decodes CSV and returns all tokens
func decodeCSV(t *testing.T, decoder *Decoder) []token.Token {
	t.Helper()
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
