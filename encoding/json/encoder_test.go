package json

import (
	"bytes"
	"strings"
	"testing"

	"github.com/arnodel/jsonstream/internal/format"
	"github.com/arnodel/jsonstream/token"
)

// TestEncoderSimpleValues tests encoding simple scalar values
func TestEncoderSimpleValues(t *testing.T) {
	tests := []struct {
		name     string
		tokens   []token.Token
		expected string
	}{
		{
			name:     "true",
			tokens:   []token.Token{token.TrueScalar},
			expected: "true",
		},
		{
			name:     "false",
			tokens:   []token.Token{token.FalseScalar},
			expected: "false",
		},
		{
			name:     "null",
			tokens:   []token.Token{token.NullScalar},
			expected: "null",
		},
		{
			name:     "integer",
			tokens:   []token.Token{token.Int64Scalar(42)},
			expected: "42",
		},
		{
			name:     "negative integer",
			tokens:   []token.Token{token.Int64Scalar(-123)},
			expected: "-123",
		},
		{
			name:     "float",
			tokens:   []token.Token{token.Float64Scalar(3.14)},
			expected: "3.14e+00", // Float64Scalar uses scientific notation
		},
		{
			name:     "string",
			tokens:   []token.Token{token.StringScalar("hello")},
			expected: `"hello"`,
		},
		{
			name:     "empty string",
			tokens:   []token.Token{token.StringScalar("")},
			expected: `""`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := strings.TrimSpace(encodeTokens(t, tt.tokens))
			if output != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, output)
			}
		})
	}
}

// TestEncoderArrays tests array encoding
func TestEncoderArrays(t *testing.T) {
	tests := []struct {
		name     string
		tokens   []token.Token
		expected string
	}{
		{
			name: "empty array",
			tokens: []token.Token{
				&token.StartArray{},
				&token.EndArray{},
			},
			expected: "[]",
		},
		{
			name: "array with one element",
			tokens: []token.Token{
				&token.StartArray{},
				token.Int64Scalar(42),
				&token.EndArray{},
			},
			expected: "[\n  42\n]",
		},
		{
			name: "array with multiple elements",
			tokens: []token.Token{
				&token.StartArray{},
				token.Int64Scalar(1),
				token.Int64Scalar(2),
				token.Int64Scalar(3),
				&token.EndArray{},
			},
			expected: "[\n  1,\n  2,\n  3\n]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := strings.TrimSpace(encodeTokens(t, tt.tokens))
			if output != tt.expected {
				t.Errorf("expected:\n%s\ngot:\n%s", tt.expected, output)
			}
		})
	}
}

// TestEncoderObjects tests object encoding
func TestEncoderObjects(t *testing.T) {
	tests := []struct {
		name     string
		tokens   []token.Token
		expected string
	}{
		{
			name: "empty object",
			tokens: []token.Token{
				&token.StartObject{},
				&token.EndObject{},
			},
			expected: "{}",
		},
		{
			name: "object with one pair",
			tokens: []token.Token{
				&token.StartObject{},
				token.StringScalar("name"),
				token.StringScalar("Alice"),
				&token.EndObject{},
			},
			expected: "{\n  \"name\": \"Alice\"\n}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := strings.TrimSpace(encodeTokens(t, tt.tokens))
			if output != tt.expected {
				t.Errorf("expected:\n%s\ngot:\n%s", tt.expected, output)
			}
		})
	}
}

// TestEncoderCompactArrays tests compact array formatting
func TestEncoderCompactArrays(t *testing.T) {
	encoder := &Encoder{
		Printer:           &format.DefaultPrinter{Writer: &bytes.Buffer{}, IndentSize: 2},
		CompactWidthLimit: 20,
	}

	// Small array should be compact
	tokens := []token.Token{
		&token.StartArray{},
		token.Int64Scalar(1),
		token.Int64Scalar(2),
		token.Int64Scalar(3),
		&token.EndArray{},
	}

	output := encodeWithEncoder(t, encoder, tokens)
	// Should be on one line with proper spacing
	if !strings.Contains(output, ", ") {
		t.Errorf("expected compact format with ', ', got: %s", output)
	}
}

// TestEncoderCompactObjects tests compact object formatting
func TestEncoderCompactObjects(t *testing.T) {
	encoder := &Encoder{
		Printer:               &format.DefaultPrinter{Writer: &bytes.Buffer{}, IndentSize: 2},
		CompactObjectMaxItems: 3,
		CompactWidthLimit:     30,
	}

	// Small object with scalar values should be compact
	tokens := []token.Token{
		&token.StartObject{},
		token.StringScalar("a"),
		token.Int64Scalar(1),
		token.StringScalar("b"),
		token.Int64Scalar(2),
		&token.EndObject{},
	}

	output := strings.TrimSpace(encodeWithEncoder(t, encoder, tokens))
	// Should be on one line (no embedded newlines)
	if strings.Contains(output, "\n") {
		t.Errorf("expected single line output for compact object, got: %s", output)
	}
}

// TestEncoderNestedStructures tests nested arrays and objects
func TestEncoderNestedStructures(t *testing.T) {
	tokens := []token.Token{
		&token.StartArray{},
		&token.StartArray{},
		token.Int64Scalar(1),
		token.Int64Scalar(2),
		&token.EndArray{},
		&token.StartArray{},
		token.Int64Scalar(3),
		token.Int64Scalar(4),
		&token.EndArray{},
		&token.EndArray{},
	}

	output := encodeTokens(t, tokens)

	// Should have proper nesting
	if !strings.Contains(output, "[\n  [\n    1") {
		t.Errorf("expected proper nesting, got:\n%s", output)
	}
}

// TestEncoderMultipleValues tests encoding multiple top-level values
func TestEncoderMultipleValues(t *testing.T) {
	tokens := []token.Token{
		token.Int64Scalar(1),
		token.StringScalar("hello"),
		token.TrueScalar,
	}

	output := encodeTokens(t, tokens)
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d: %v", len(lines), lines)
	}
}

// TestEncoderSingleLine tests encoding with no indentation (single line)
func TestEncoderSingleLine(t *testing.T) {
	var buf bytes.Buffer
	encoder := &Encoder{
		Printer: &format.DefaultPrinter{Writer: &buf, IndentSize: -1}, // Negative indent = single line
	}

	tokens := []token.Token{
		&token.StartArray{},
		token.Int64Scalar(1),
		token.Int64Scalar(2),
		token.Int64Scalar(3),
		&token.EndArray{},
	}

	output := strings.TrimSpace(encodeWithEncoder(t, encoder, tokens))
	if strings.Contains(output, "\n") {
		t.Errorf("expected single line, got:\n%s", output)
	}
}

// TestEncoderElision tests elision markers
func TestEncoderElision(t *testing.T) {
	tests := []struct {
		name     string
		tokens   []token.Token
		expected string
	}{
		{
			name: "array with elision",
			tokens: []token.Token{
				&token.StartArray{},
				token.Int64Scalar(1),
				&token.Elision{},
				&token.EndArray{},
			},
			expected: "...",
		},
		{
			name: "object with elision",
			tokens: []token.Token{
				&token.StartObject{},
				token.StringScalar("key"),
				token.StringScalar("value"),
				&token.Elision{},
				&token.EndObject{},
			},
			expected: "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := encodeTokens(t, tt.tokens)
			if !strings.Contains(output, tt.expected) {
				t.Errorf("expected output to contain %q, got:\n%s", tt.expected, output)
			}
		})
	}
}

// TestEncoderComplexDocument tests a realistic document
func TestEncoderComplexDocument(t *testing.T) {
	tokens := []token.Token{
		&token.StartObject{},
		token.StringScalar("name"),
		token.StringScalar("John"),
		token.StringScalar("age"),
		token.Int64Scalar(30),
		token.StringScalar("active"),
		token.TrueScalar,
		token.StringScalar("tags"),
		&token.StartArray{},
		token.StringScalar("admin"),
		token.StringScalar("user"),
		&token.EndArray{},
		&token.EndObject{},
	}

	output := encodeTokens(t, tokens)

	// Verify it contains expected elements
	requiredParts := []string{"{", "}", "\"name\"", "\"John\"", "\"age\"", "30", "true", "[", "]", "\"admin\"", "\"user\""}
	for _, part := range requiredParts {
		if !strings.Contains(output, part) {
			t.Errorf("expected output to contain %q, got:\n%s", part, output)
		}
	}
}

// Helper functions

// encodeTokens encodes tokens using a default encoder and returns the output
func encodeTokens(t *testing.T, tokens []token.Token) string {
	t.Helper()
	var buf bytes.Buffer
	encoder := &Encoder{
		Printer: &format.DefaultPrinter{Writer: &buf, IndentSize: 2},
	}
	return encodeWithEncoder(t, encoder, tokens)
}

// encodeWithEncoder encodes tokens with a specific encoder
func encodeWithEncoder(t *testing.T, encoder *Encoder, tokens []token.Token) string {
	t.Helper()

	// Get the buffer from the encoder's printer
	printer, ok := encoder.Printer.(*format.DefaultPrinter)
	if !ok {
		t.Fatal("expected DefaultPrinter")
	}
	buf, ok := printer.Writer.(*bytes.Buffer)
	if !ok {
		t.Fatal("expected bytes.Buffer")
	}
	buf.Reset()

	// Create a channel and send tokens
	tokenChan := make(chan token.Token, len(tokens))
	for _, tok := range tokens {
		tokenChan <- tok
	}
	close(tokenChan)

	// Consume the tokens
	err := encoder.Consume(tokenChan)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}

	return buf.String()
}
