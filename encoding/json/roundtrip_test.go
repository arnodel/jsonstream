package json

import (
	"bytes"
	"strings"
	"testing"

	"github.com/arnodel/jsonstream/internal/format"
	"github.com/arnodel/jsonstream/token"
)

// TestRoundtripSimpleValues tests encoding then decoding simple values
func TestRoundtripSimpleValues(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"true", "true"},
		{"false", "false"},
		{"null", "null"},
		{"integer", "42"},
		{"negative", "-123"},
		{"string", `"hello"`},
		{"empty string", `""`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := roundtrip(t, tt.input)
			// Normalize whitespace for comparison
			expected := strings.TrimSpace(tt.input)
			actual := strings.TrimSpace(output)

			if actual != expected {
				t.Errorf("roundtrip mismatch:\ninput:  %s\noutput: %s", expected, actual)
			}
		})
	}
}

// TestRoundtripArrays tests encoding then decoding arrays
func TestRoundtripArrays(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", "[]"},
		{"single element", "[42]"},
		{"multiple elements", "[1, 2, 3]"},
		{"mixed types", `[1, "hello", true, null]`},
		{"nested", "[[1, 2], [3, 4]]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := roundtrip(t, tt.input)
			// Verify the output is valid JSON
			tokens := decodeString(t, output)
			if len(tokens) == 0 {
				t.Error("roundtrip produced no tokens")
			}
		})
	}
}

// TestRoundtripObjects tests encoding then decoding objects
func TestRoundtripObjects(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", "{}"},
		{"single pair", `{"name": "Alice"}`},
		{"multiple pairs", `{"name": "Alice", "age": 30}`},
		{"nested", `{"user": {"name": "Alice"}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := roundtrip(t, tt.input)
			// Verify the output is valid JSON
			tokens := decodeString(t, output)
			if len(tokens) == 0 {
				t.Error("roundtrip produced no tokens")
			}
		})
	}
}

// TestRoundtripComplexDocument tests a realistic document
func TestRoundtripComplexDocument(t *testing.T) {
	input := `{
		"name": "John Doe",
		"age": 30,
		"email": "john@example.com",
		"address": {
			"street": "123 Main St",
			"city": "Springfield"
		},
		"phones": [
			{"type": "home", "number": "555-1234"},
			{"type": "work", "number": "555-5678"}
		],
		"active": true
	}`

	output := roundtrip(t, input)

	// Verify the output is valid JSON
	tokens := decodeString(t, output)
	if len(tokens) < 10 {
		t.Errorf("expected at least 10 tokens, got %d", len(tokens))
	}

	// Verify it contains expected values
	outputStr := output
	if !strings.Contains(outputStr, "John Doe") {
		t.Error("output missing 'John Doe'")
	}
	if !strings.Contains(outputStr, "Springfield") {
		t.Error("output missing 'Springfield'")
	}
}

// TestRoundtripMultipleValues tests multiple top-level values
func TestRoundtripMultipleValues(t *testing.T) {
	input := `42 "hello" true`

	output := roundtrip(t, input)

	// Should be able to decode all three values
	tokens := decodeString(t, output)
	if len(tokens) != 3 {
		t.Errorf("expected 3 tokens, got %d", len(tokens))
	}
}

// TestRoundtripPreservesStructure tests that structure is preserved
func TestRoundtripPreservesStructure(t *testing.T) {
	input := `{"a": [1, 2], "b": {"c": 3}}`

	// Decode original
	inputTokens := decodeString(t, input)

	// Roundtrip
	output := roundtrip(t, input)

	// Decode roundtripped
	outputTokens := decodeString(t, output)

	// Should have same number of tokens (structure preserved)
	if len(inputTokens) != len(outputTokens) {
		t.Errorf("token count mismatch: input=%d, output=%d", len(inputTokens), len(outputTokens))
	}
}

// TestRoundtripLargeArray tests handling of large arrays
func TestRoundtripLargeArray(t *testing.T) {
	// Build a large array
	var sb strings.Builder
	sb.WriteString("[")
	for i := 0; i < 100; i++ {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("42")
	}
	sb.WriteString("]")

	input := sb.String()
	output := roundtrip(t, input)

	// Verify it's valid
	tokens := decodeString(t, output)
	// Should have: StartArray + 100 numbers + EndArray = 102 tokens
	expected := 102
	if len(tokens) != expected {
		t.Errorf("expected %d tokens, got %d", expected, len(tokens))
	}
}

// TestRoundtripDeepNesting tests deeply nested structures
func TestRoundtripDeepNesting(t *testing.T) {
	depth := 10
	var sb strings.Builder

	// Build deeply nested arrays
	for i := 0; i < depth; i++ {
		sb.WriteString("[")
	}
	sb.WriteString("42")
	for i := 0; i < depth; i++ {
		sb.WriteString("]")
	}

	input := sb.String()
	output := roundtrip(t, input)

	// Verify structure preserved
	tokens := decodeString(t, output)
	expected := depth*2 + 1
	if len(tokens) != expected {
		t.Errorf("expected %d tokens, got %d", expected, len(tokens))
	}
}

// TestRoundtripWithCompactFormat tests roundtrip with compact encoding
func TestRoundtripWithCompactFormat(t *testing.T) {
	input := `[1, 2, 3, 4, 5]`

	// Decode
	decoder := NewDecoder(strings.NewReader(input))
	tokenChan := make(chan token.Token, 100)

	go func() {
		decoder.Produce(tokenChan)
		close(tokenChan)
	}()

	// Encode with compact settings
	var buf bytes.Buffer
	encoder := &Encoder{
		Printer:           &format.DefaultPrinter{Writer: &buf, IndentSize: -1}, // Single line
		CompactWidthLimit: 50,
	}

	err := encoder.Consume(tokenChan)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}

	output := buf.String()

	// Should be compact (single line)
	if strings.Count(output, "\n") > 1 {
		t.Errorf("expected single line output, got:\n%s", output)
	}

	// Should be valid JSON
	tokens := decodeString(t, strings.TrimSpace(output))
	if len(tokens) == 0 {
		t.Error("compact roundtrip produced no tokens")
	}
}

// TestRoundtripSpecialCharacters tests strings with special characters
func TestRoundtripSpecialCharacters(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"quotes", `"hello \"world\""`},
		{"backslash", `"hello\\world"`},
		{"newline", `"hello\nworld"`},
		{"tab", `"hello\tworld"`},
		{"unicode", `"hello\u0041world"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := roundtrip(t, tt.input)
			// Should be valid JSON
			tokens := decodeString(t, output)
			if len(tokens) != 1 {
				t.Errorf("expected 1 token, got %d", len(tokens))
			}
		})
	}
}

// Helper function

// roundtrip decodes JSON and then encodes it again
func roundtrip(t *testing.T, input string) string {
	t.Helper()

	// Decode
	decoder := NewDecoder(strings.NewReader(input))
	tokenChan := make(chan token.Token, 100)

	go func() {
		err := decoder.Produce(tokenChan)
		if err != nil {
			t.Errorf("decode error: %v", err)
		}
		close(tokenChan)
	}()

	// Encode
	var buf bytes.Buffer
	encoder := &Encoder{
		Printer: &format.DefaultPrinter{Writer: &buf, IndentSize: 2},
	}

	err := encoder.Consume(tokenChan)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}

	return buf.String()
}
