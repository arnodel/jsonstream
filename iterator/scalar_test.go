package iterator

import (
	"testing"

	"github.com/arnodel/jsonstream/token"
)

func TestScalarTypes(t *testing.T) {
	tests := []struct {
		name  string
		value any
		check func(*testing.T, *token.Scalar)
	}{
		{
			name:  "string",
			value: "hello",
			check: func(t *testing.T, s *token.Scalar) {
				if s.ToString() != "hello" {
					t.Errorf("got %q, want %q", s.ToString(), "hello")
				}
				if s.Type() != token.String {
					t.Errorf("got type %d, want %d", s.Type(), token.String)
				}
			},
		},
		{
			name:  "integer",
			value: 42,
			check: func(t *testing.T, s *token.Scalar) {
				if s.Type() != token.Number {
					t.Errorf("got type %d, want %d", s.Type(), token.Number)
				}
				val := s.ToGo()
				num, ok := val.(float64) // JSON numbers are decoded as float64
				assertTrue(t, ok, "should convert to float64")
				if int64(num) != 42 {
					t.Errorf("got %d, want 42", int64(num))
				}
			},
		},
		{
			name:  "float",
			value: 3.14,
			check: func(t *testing.T, s *token.Scalar) {
				if s.Type() != token.Number {
					t.Errorf("got type %d, want %d", s.Type(), token.Number)
				}
			},
		},
		{
			name:  "true",
			value: true,
			check: func(t *testing.T, s *token.Scalar) {
				if s.Type() != token.Boolean {
					t.Errorf("got type %d, want %d", s.Type(), token.Boolean)
				}
				val := s.ToGo()
				b, ok := val.(bool)
				assertTrue(t, ok, "should convert to bool")
				if b != true {
					t.Errorf("got %v, want true", b)
				}
			},
		},
		{
			name:  "false",
			value: false,
			check: func(t *testing.T, s *token.Scalar) {
				if s.Type() != token.Boolean {
					t.Errorf("got type %d, want %d", s.Type(), token.Boolean)
				}
				val := s.ToGo()
				b, ok := val.(bool)
				assertTrue(t, ok, "should convert to bool")
				if b != false {
					t.Errorf("got %v, want false", b)
				}
			},
		},
		{
			name:  "null",
			value: nil,
			check: func(t *testing.T, s *token.Scalar) {
				if s.Type() != token.Null {
					t.Errorf("got type %d, want %d", s.Type(), token.Null)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scalar := makeScalar(t, tt.value)
			tt.check(t, scalar.Scalar())
		})
	}
}

func TestScalarTypeAssertions(t *testing.T) {
	scalar := makeScalar(t, "test")

	// AsScalar should succeed
	s, ok := scalar.AsScalar()
	assertTrue(t, ok, "AsScalar should return true")
	assertNotNil(t, s, "AsScalar should return non-nil scalar")

	// AsArray should fail
	_, ok = scalar.AsArray()
	assertFalse(t, ok, "AsArray should return false for scalar")

	// AsObject should fail
	_, ok = scalar.AsObject()
	assertFalse(t, ok, "AsObject should return false for scalar")
}

func TestScalarClone(t *testing.T) {
	original := makeScalar(t, "hello")

	// Clone should return the same scalar
	cloned, detach := original.Clone()

	// Detach should be nil for scalars (they're immutable)
	if detach != nil {
		t.Error("scalar Clone should return nil detach function")
	}

	// Cloned should be the same scalar (immutable)
	assertTrue(t, cloned == original, "scalar clone should return same instance")

	// Both should have same value
	s1, _ := original.AsScalar()
	s2, _ := cloned.AsScalar()
	if s1.ToString() != s2.ToString() {
		t.Errorf("got %q, want %q", s1.ToString(), s2.ToString())
	}
}

func TestScalarDiscard(t *testing.T) {
	scalar := makeScalar(t, 42)

	// Discard should be a no-op (no panic)
	scalar.Discard()

	// Value should still be accessible after discard
	s, _ := scalar.AsScalar()
	if s.Type() != token.Number {
		t.Errorf("got type %d, want %d", s.Type(), token.Number)
	}
}

func TestScalarCopy(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{"string", "hello"},
		{"int", 42},
		{"float", 3.14},
		{"true", true},
		{"false", false},
		{"null", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scalar := makeScalar(t, tt.value)

			// Copy to a channel
			tokens := make(chan token.Token, 10)
			done := make(chan bool)

			go func() {
				scalar.Copy(token.ChannelWriteStream(tokens))
				close(tokens)
				done <- true
			}()

			// Collect tokens
			var collected []token.Token
			for tok := range tokens {
				collected = append(collected, tok)
			}
			<-done

			// Should have exactly one token
			assertEqual(t, len(collected), 1)

			// Verify it's a scalar token
			scalar2, ok := collected[0].(*token.Scalar)
			assertTrue(t, ok, "copied token should be scalar")
			assertTrue(t, scalar.Scalar().Equal(scalar2), "copied scalar should be equal")
		})
	}
}

func TestScalarEqual(t *testing.T) {
	tests := []struct {
		name     string
		a, b     any
		expected bool
	}{
		{"same string", "hello", "hello", true},
		{"different string", "hello", "world", false},
		{"same int", 42, 42, true},
		{"different int", 42, 43, false},
		{"same float", 3.14, 3.14, true},
		{"different float", 3.14, 2.71, false},
		{"same bool true", true, true, true},
		{"same bool false", false, false, true},
		{"different bool", true, false, false},
		{"same null", nil, nil, true},
		{"string vs int", "42", 42, false},
		{"int vs float", 42, 42.0, true}, // Same numeric value
		{"bool vs int", true, 1, false},
		{"null vs int", nil, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valA := makeScalar(t, tt.a)
			valB := makeScalar(t, tt.b)

			got := valA.Equal(valB)
			assertEqual(t, got, tt.expected)

			// Equal should be symmetric
			got2 := valB.Equal(valA)
			assertEqual(t, got2, tt.expected)
		})
	}
}

func TestScalarEqualDifferentTypes(t *testing.T) {
	scalar := makeScalar(t, "test")
	arr := makeTestArray(t, 1, 2, 3)
	obj := makeTestObject(t, map[string]any{"key": "value"})

	// Scalar vs Array should be false
	assertFalse(t, scalar.Equal(arr), "scalar should not equal array")
	assertFalse(t, arr.Equal(scalar), "array should not equal scalar")

	// Scalar vs Object should be false
	assertFalse(t, scalar.Equal(obj), "scalar should not equal object")
	assertFalse(t, obj.Equal(scalar), "object should not equal scalar")
}

func TestScalarEqualNil(t *testing.T) {
	scalar := makeScalar(t, "test")

	// Equal with nil should not panic and should return false
	result := scalar.Equal(nil)
	assertFalse(t, result, "scalar.Equal(nil) should be false")
}
