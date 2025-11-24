package iterator

import (
	"testing"

	"github.com/arnodel/jsonstream/token"
)

// identityTransformer copies values unchanged
type identityTransformer struct{}

func (t *identityTransformer) TransformValue(value Value, out token.WriteStream) {
	value.Copy(out)
}

// doubleNumbersTransformer doubles all numbers, copies everything else
type doubleNumbersTransformer struct{}

func (t *doubleNumbersTransformer) TransformValue(value Value, out token.WriteStream) {
	if scalar, ok := value.AsScalar(); ok {
		if scalar.Type() == token.Number {
			// Double the number
			num := scalar.ToGo().(float64)
			out.Put(token.Float64Scalar(num * 2))
			return
		}
		// Copy other scalars unchanged
		value.Copy(out)
		return
	}

	if arr, ok := value.AsArray(); ok {
		out.Put(&token.StartArray{})
		for arr.Advance() {
			t.TransformValue(arr.CurrentValue(), out)
		}
		out.Put(&token.EndArray{})
		return
	}

	if obj, ok := value.AsObject(); ok {
		out.Put(&token.StartObject{})
		for obj.Advance() {
			key, val := obj.CurrentKeyVal()
			out.Put(key)
			t.TransformValue(val, out)
		}
		out.Put(&token.EndObject{})
		return
	}
}

// filterNullsTransformer skips null values
type filterNullsTransformer struct{}

func (t *filterNullsTransformer) TransformValue(value Value, out token.WriteStream) {
	if scalar, ok := value.AsScalar(); ok {
		if scalar.Type() == token.Null {
			return // Skip nulls
		}
	}
	value.Copy(out)
}

// wrapInArrayTransformer wraps each value in an array
type wrapInArrayTransformer struct{}

func (t *wrapInArrayTransformer) TransformValue(value Value, out token.WriteStream) {
	out.Put(&token.StartArray{})
	value.Copy(out)
	out.Put(&token.EndArray{})
}

// countingTransformer counts values without outputting them
type countingTransformer struct {
	count int
}

func (t *countingTransformer) TransformValue(value Value, out token.WriteStream) {
	t.count++
	// Don't write anything to output
}

// TestTransformerIdentity tests identity transformer (no-op)
func TestTransformerIdentity(t *testing.T) {
	tests := []struct {
		name   string
		input  []any
		verify func(*testing.T, []token.Token)
	}{
		{
			name:  "single scalar",
			input: []any{42},
			verify: func(t *testing.T, tokens []token.Token) {
				if len(tokens) != 1 {
					t.Errorf("expected 1 token, got %d", len(tokens))
				}
				scalar, ok := tokens[0].(*token.Scalar)
				assertTrue(t, ok, "expected scalar token")
				num := scalar.ToGo().(float64)
				if int64(num) != 42 {
					t.Errorf("expected 42, got %d", int64(num))
				}
			},
		},
		{
			name:  "array",
			input: []any{[]any{1, 2, 3}},
			verify: func(t *testing.T, tokens []token.Token) {
				// Should have: StartArray, 3 scalars, EndArray
				if len(tokens) != 5 {
					t.Errorf("expected 5 tokens, got %d", len(tokens))
				}
				_, ok := tokens[0].(*token.StartArray)
				assertTrue(t, ok, "first token should be StartArray")
				_, ok = tokens[4].(*token.EndArray)
				assertTrue(t, ok, "last token should be EndArray")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transformer := &identityTransformer{}
			tokens := runTransformer(t, transformer, tt.input...)
			tt.verify(t, tokens)
		})
	}
}

// TestTransformerDoubleNumbers tests transformer that modifies scalar values
func TestTransformerDoubleNumbers(t *testing.T) {
	transformer := &doubleNumbersTransformer{}

	tests := []struct {
		name     string
		input    []any
		expected []any
	}{
		{
			name:     "single number",
			input:    []any{5},
			expected: []any{10.0},
		},
		{
			name:     "multiple numbers",
			input:    []any{1, 2, 3},
			expected: []any{2.0, 4.0, 6.0},
		},
		{
			name:     "mixed types",
			input:    []any{5, "hello", 10},
			expected: []any{10.0, "hello", 20.0},
		},
		{
			name:     "string unchanged",
			input:    []any{"test"},
			expected: []any{"test"},
		},
		{
			name:     "boolean unchanged",
			input:    []any{true, false},
			expected: []any{true, false},
		},
		{
			name:     "null unchanged",
			input:    []any{nil},
			expected: []any{nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := runTransformer(t, transformer, tt.input...)
			verifyTokenValues(t, tokens, tt.expected)
		})
	}
}

// TestTransformerFilterNulls tests transformer that filters out values
func TestTransformerFilterNulls(t *testing.T) {
	transformer := &filterNullsTransformer{}

	tests := []struct {
		name     string
		input    []any
		expected []any
	}{
		{
			name:     "no nulls",
			input:    []any{1, 2, 3},
			expected: []any{1, 2, 3},
		},
		{
			name:     "all nulls",
			input:    []any{nil, nil, nil},
			expected: []any{},
		},
		{
			name:     "mixed with nulls",
			input:    []any{1, nil, 2, nil, 3},
			expected: []any{1, 2, 3},
		},
		{
			name:     "null at start",
			input:    []any{nil, 1, 2},
			expected: []any{1, 2},
		},
		{
			name:     "null at end",
			input:    []any{1, 2, nil},
			expected: []any{1, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := runTransformer(t, transformer, tt.input...)
			verifyTokenValues(t, tokens, tt.expected)
		})
	}
}

// TestTransformerWrapInArray tests transformer that wraps values
func TestTransformerWrapInArray(t *testing.T) {
	transformer := &wrapInArrayTransformer{}

	// Single scalar wrapped in array
	tokens := runTransformer(t, transformer, 42)

	// Should have: StartArray, scalar, EndArray, for each input value
	// Input is 1 value, so output is 1 wrapped array
	if len(tokens) != 3 {
		t.Errorf("expected 3 tokens, got %d", len(tokens))
	}

	_, ok := tokens[0].(*token.StartArray)
	assertTrue(t, ok, "first token should be StartArray")

	scalar, ok := tokens[1].(*token.Scalar)
	assertTrue(t, ok, "second token should be Scalar")
	num := scalar.ToGo().(float64)
	if int64(num) != 42 {
		t.Errorf("expected 42, got %d", int64(num))
	}

	_, ok = tokens[2].(*token.EndArray)
	assertTrue(t, ok, "third token should be EndArray")
}

// TestTransformerMultipleValues tests transformer with multiple input values
func TestTransformerMultipleValues(t *testing.T) {
	transformer := &identityTransformer{}

	// Three separate values
	tokens := runTransformer(t, transformer, 1, 2, 3)

	// Should have 3 scalar tokens
	if len(tokens) != 3 {
		t.Errorf("expected 3 tokens, got %d", len(tokens))
	}

	for i, tok := range tokens {
		scalar, ok := tok.(*token.Scalar)
		assertTrue(t, ok, "token should be scalar")
		num := scalar.ToGo().(float64)
		if int64(num) != int64(i+1) {
			t.Errorf("token %d: expected %d, got %d", i, i+1, int64(num))
		}
	}
}

// TestTransformerWithArrays tests transformer with array values
func TestTransformerWithArrays(t *testing.T) {
	transformer := &doubleNumbersTransformer{}

	// Array containing numbers - should double them
	tokens := runTransformer(t, transformer, []any{1, 2, 3})

	// Should have: StartArray, 3 doubled scalars, EndArray
	if len(tokens) != 5 {
		t.Errorf("expected 5 tokens, got %d", len(tokens))
	}

	_, ok := tokens[0].(*token.StartArray)
	assertTrue(t, ok, "first token should be StartArray")

	// Check doubled values
	for i := 1; i <= 3; i++ {
		scalar, ok := tokens[i].(*token.Scalar)
		assertTrue(t, ok, "token should be scalar")
		num := scalar.ToGo().(float64)
		expected := float64(i * 2)
		if num != expected {
			t.Errorf("element %d: expected %v, got %v", i, expected, num)
		}
	}

	_, ok = tokens[4].(*token.EndArray)
	assertTrue(t, ok, "last token should be EndArray")
}

// TestTransformerWithObjects tests transformer with object values
func TestTransformerWithObjects(t *testing.T) {
	transformer := &doubleNumbersTransformer{}

	// Object containing numbers - should double them
	tokens := runTransformer(t, transformer, map[string]any{
		"a": 10,
		"b": 20,
	})

	// Should have: StartObject, key "a", value 20, key "b", value 40, EndObject
	// (At least 6 tokens, order may vary due to map)
	if len(tokens) < 6 {
		t.Errorf("expected at least 6 tokens, got %d", len(tokens))
	}

	_, ok := tokens[0].(*token.StartObject)
	assertTrue(t, ok, "first token should be StartObject")

	_, ok = tokens[len(tokens)-1].(*token.EndObject)
	assertTrue(t, ok, "last token should be EndObject")

	// Verify that numbers are doubled (checking all number tokens)
	for _, tok := range tokens[1 : len(tokens)-1] {
		if scalar, ok := tok.(*token.Scalar); ok {
			if scalar.Type() == token.Number {
				num := scalar.ToGo().(float64)
				// Should be 20 or 40 (10*2 or 20*2)
				if num != 20 && num != 40 {
					t.Errorf("expected doubled value (20 or 40), got %v", num)
				}
			}
		}
	}
}

// TestTransformerWithNestedStructures tests transformer with nested values
func TestTransformerWithNestedStructures(t *testing.T) {
	transformer := &doubleNumbersTransformer{}

	// Nested structure: array of objects
	tokens := runTransformer(t, transformer, []any{
		map[string]any{"value": 5},
		map[string]any{"value": 10},
	})

	// Verify structure is preserved and numbers are doubled
	_, ok := tokens[0].(*token.StartArray)
	assertTrue(t, ok, "first token should be StartArray")

	// Find all number tokens and verify they're doubled
	foundDoubled := false
	for _, tok := range tokens {
		if scalar, ok := tok.(*token.Scalar); ok {
			if scalar.Type() == token.Number {
				num := scalar.ToGo().(float64)
				// Should be 10 or 20 (5*2 or 10*2)
				if num == 10 || num == 20 {
					foundDoubled = true
				}
			}
		}
	}
	assertTrue(t, foundDoubled, "should find doubled numbers in nested structure")
}

// TestTransformerCounting tests transformer that doesn't output
func TestTransformerCounting(t *testing.T) {
	transformer := &countingTransformer{}

	// Transform 5 values
	tokens := runTransformer(t, transformer, 1, 2, 3, 4, 5)

	// Should have no output tokens
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens, got %d", len(tokens))
	}

	// But should have counted 5 values
	if transformer.count != 5 {
		t.Errorf("expected count=5, got %d", transformer.count)
	}
}

// TestTransformerEmptyInput tests transformer with no input
func TestTransformerEmptyInput(t *testing.T) {
	transformer := &identityTransformer{}

	tokens := runTransformer(t, transformer) // No values

	// Should have no output
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens, got %d", len(tokens))
	}
}

// TestAsStreamTransformer tests the AsStreamTransformer adapter function
func TestAsStreamTransformer(t *testing.T) {
	valueTransformer := &doubleNumbersTransformer{}
	streamTransformer := AsStreamTransformer(valueTransformer)

	// Create input channel with tokens
	in := make(chan token.Token, 10)
	in <- token.Int64Scalar(5)
	in <- token.Int64Scalar(10)
	close(in)

	// Create output channel
	out := make(chan token.Token, 10)
	outStream := token.ChannelWriteStream(out)

	// Run transformation
	streamTransformer.Transform(in, outStream)
	close(out)

	// Collect output
	var tokens []token.Token
	for tok := range out {
		tokens = append(tokens, tok)
	}

	// Should have 2 doubled numbers
	if len(tokens) != 2 {
		t.Errorf("expected 2 tokens, got %d", len(tokens))
	}

	for i, tok := range tokens {
		scalar, ok := tok.(*token.Scalar)
		assertTrue(t, ok, "token should be scalar")
		num := scalar.ToGo().(float64)
		expected := float64((i+1) * 5 * 2) // 10 or 20
		if num != expected {
			t.Errorf("token %d: expected %v, got %v", i, expected, num)
		}
	}
}

// runTransformer helper runs a transformer on input values and returns output tokens
func runTransformer(t *testing.T, transformer ValueTransformer, values ...any) []token.Token {
	t.Helper()

	// Create iterator from values
	it := makeIterator(t, values...)

	// Create output channel
	tokens := make(chan token.Token, 100)
	out := token.ChannelWriteStream(tokens)

	// Transform all values
	for it.Advance() {
		transformer.TransformValue(it.CurrentValue(), out)
	}

	close(tokens)

	// Collect output tokens
	var result []token.Token
	for tok := range tokens {
		result = append(result, tok)
	}

	return result
}

// verifyTokenValues checks that tokens match expected values
func verifyTokenValues(t *testing.T, tokens []token.Token, expected []any) {
	t.Helper()

	if len(tokens) != len(expected) {
		t.Errorf("expected %d tokens, got %d", len(expected), len(tokens))
		return
	}

	for i, expectedVal := range expected {
		scalar, ok := tokens[i].(*token.Scalar)
		if !ok {
			t.Errorf("token %d: expected scalar, got %T", i, tokens[i])
			continue
		}

		actualVal := scalar.ToGo()

		// Type-specific comparison
		switch ev := expectedVal.(type) {
		case int:
			num, ok := actualVal.(float64)
			if !ok {
				t.Errorf("token %d: expected number, got %T", i, actualVal)
				continue
			}
			if int64(num) != int64(ev) {
				t.Errorf("token %d: expected %d, got %d", i, ev, int64(num))
			}
		case float64:
			num, ok := actualVal.(float64)
			if !ok {
				t.Errorf("token %d: expected number, got %T", i, actualVal)
				continue
			}
			if num != ev {
				t.Errorf("token %d: expected %v, got %v", i, ev, num)
			}
		case string:
			str, ok := actualVal.(string)
			if !ok {
				t.Errorf("token %d: expected string, got %T", i, actualVal)
				continue
			}
			if str != ev {
				t.Errorf("token %d: expected %q, got %q", i, ev, str)
			}
		case bool:
			b, ok := actualVal.(bool)
			if !ok {
				t.Errorf("token %d: expected bool, got %T", i, actualVal)
				continue
			}
			if b != ev {
				t.Errorf("token %d: expected %v, got %v", i, ev, b)
			}
		case nil:
			if actualVal != nil {
				t.Errorf("token %d: expected nil, got %v", i, actualVal)
			}
		default:
			t.Errorf("token %d: unsupported expected type %T", i, expectedVal)
		}
	}
}
