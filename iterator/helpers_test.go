package iterator

import (
	"fmt"
	"strings"
	"testing"

	"github.com/arnodel/jsonstream/token"
)

// makeTokenStream creates a token.ReadStream from Go values
// Supports: primitives (string, int, float64, bool, nil), []any for arrays, map[string]any for objects
func makeTokenStream(t *testing.T, values ...any) token.ReadStream {
	t.Helper()
	tokens := make([]token.Token, 0)
	for _, v := range values {
		tokens = append(tokens, valueToTokens(t, v)...)
	}
	return token.NewSliceReadStream(tokens)
}

// valueToTokens converts a Go value to JSON tokens
func valueToTokens(t *testing.T, v any) []token.Token {
	t.Helper()
	switch val := v.(type) {
	case string:
		return []token.Token{token.StringScalar(val)}
	case int:
		return []token.Token{token.Int64Scalar(int64(val))}
	case int64:
		return []token.Token{token.Int64Scalar(val)}
	case float64:
		return []token.Token{token.Float64Scalar(val)}
	case bool:
		if val {
			return []token.Token{token.TrueScalar}
		}
		return []token.Token{token.FalseScalar}
	case nil:
		return []token.Token{token.NullScalar}
	case []any:
		tokens := []token.Token{&token.StartArray{}}
		for _, item := range val {
			tokens = append(tokens, valueToTokens(t, item)...)
		}
		tokens = append(tokens, &token.EndArray{})
		return tokens
	case map[string]any:
		tokens := []token.Token{&token.StartObject{}}
		for k, v := range val {
			tokens = append(tokens, token.StringScalar(k))
			tokens = append(tokens, valueToTokens(t, v)...)
		}
		tokens = append(tokens, &token.EndObject{})
		return tokens
	default:
		t.Fatalf("unsupported value type: %T", v)
		return nil
	}
}

// jsonTokenStream creates a token stream from a JSON string
// Note: This uses internal/jsonpath/parser instead of encoding/json to avoid import cycles
func jsonTokenStream(t *testing.T, jsonStr string) token.ReadStream {
	t.Helper()
	// For now, we'll implement a simple JSON parser for tests
	// This is a simplified version that handles basic JSON
	t.Skip("jsonTokenStream not yet implemented to avoid import cycles")
	return nil
}

// makeIterator creates an iterator from Go values
func makeIterator(t *testing.T, values ...any) *Iterator {
	t.Helper()
	return New(makeTokenStream(t, values...))
}

// collectValues collects all values from an iterator into a slice
func collectValues(t *testing.T, it *Iterator) []Value {
	t.Helper()
	var values []Value
	var detachFuncs []func()

	for it.Advance() {
		val := it.CurrentValue()
		// Clone to preserve the value
		cloned, detach := val.Clone()
		if detach != nil {
			detachFuncs = append(detachFuncs, detach)
		}
		values = append(values, cloned)
	}

	// Detach all clones at the end
	for _, detach := range detachFuncs {
		detach()
	}

	return values
}

// assertPanics verifies that a function panics with a message containing expectedMsg
func assertPanics(t *testing.T, expectedMsg string, f func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			msg := fmt.Sprintf("%v", r)
			if !strings.Contains(msg, expectedMsg) {
				t.Errorf("panic message %q does not contain %q", msg, expectedMsg)
			}
		} else {
			t.Errorf("expected panic with message containing %q, but no panic occurred", expectedMsg)
		}
	}()
	f()
}

// makeScalar creates a Scalar value from a Go value
func makeScalar(t *testing.T, v any) *Scalar {
	t.Helper()
	it := makeIterator(t, v)
	if !it.Advance() {
		t.Fatal("expected iterator to have a value")
	}
	value := it.CurrentValue()
	scalar, ok := value.(*Scalar)
	if !ok {
		t.Fatalf("expected scalar value, got %T", value)
	}
	return scalar
}

// makeTestArray creates an Array value from Go values
func makeTestArray(t *testing.T, items ...any) *Array {
	t.Helper()
	it := makeIterator(t, items)
	if !it.Advance() {
		t.Fatal("expected iterator to have a value")
	}
	arr, ok := it.CurrentValue().AsArray()
	if !ok {
		t.Fatalf("expected array value, got %T", it.CurrentValue())
	}
	return arr
}

// makeTestObject creates an Object value from key-value pairs
func makeTestObject(t *testing.T, pairs map[string]any) *Object {
	t.Helper()
	it := makeIterator(t, pairs)
	if !it.Advance() {
		t.Fatal("expected iterator to have a value")
	}
	obj, ok := it.CurrentValue().AsObject()
	if !ok {
		t.Fatalf("expected object value, got %T", it.CurrentValue())
	}
	return obj
}

// tokensToString converts tokens to a readable string for debugging
func tokensToString(tokens []token.Token) string {
	var parts []string
	for _, tok := range tokens {
		parts = append(parts, fmt.Sprintf("%v", tok))
	}
	return strings.Join(parts, ", ")
}

// assertEqual is a simple assertion helper
func assertEqual(t *testing.T, got, want any) {
	t.Helper()
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

// assertTrue is a simple assertion helper for booleans
func assertTrue(t *testing.T, condition bool, msg string) {
	t.Helper()
	if !condition {
		t.Error(msg)
	}
}

// assertFalse is a simple assertion helper for booleans
func assertFalse(t *testing.T, condition bool, msg string) {
	t.Helper()
	if condition {
		t.Error(msg)
	}
}

// assertNil checks if a value is nil
func assertNil(t *testing.T, v any, msg string) {
	t.Helper()
	if v != nil {
		t.Errorf("%s: got %v, want nil", msg, v)
	}
}

// assertNotNil checks if a value is not nil
func assertNotNil(t *testing.T, v any, msg string) {
	t.Helper()
	if v == nil {
		t.Errorf("%s: got nil, want non-nil", msg)
	}
}
