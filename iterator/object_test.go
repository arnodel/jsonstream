package iterator

import (
	"testing"

	"github.com/arnodel/jsonstream/token"
)

func TestObjectEmpty(t *testing.T) {
	obj := makeTestObject(t, map[string]any{})

	// Empty object should not advance
	assertFalse(t, obj.Advance(), "empty object should not advance")
	// Done check: !obj.Advance(), "empty object should be done")
	assertFalse(t, obj.Elided(), "empty object should not be elided")
}

func TestObjectSinglePair(t *testing.T) {
	obj := makeTestObject(t, map[string]any{"name": "Alice"})

	// Should advance once
	assertTrue(t, obj.Advance(), "should advance to first pair")

	key, val := obj.CurrentKeyVal()
	if key.ToString() != "name" {
		t.Errorf("got key %q, want %q", key.ToString(), "name")
	}

	s, ok := val.AsScalar()
	assertTrue(t, ok, "value should be scalar")
	if s.ToString() != "Alice" {
		t.Errorf("got value %q, want %q", s.ToString(), "Alice")
	}

	// Should not advance again
	assertFalse(t, obj.Advance(), "should not advance past end")
	// Done check: !obj.Advance(), "should be done after exhaustion")
}

func TestObjectMultiplePairs(t *testing.T) {
	obj := makeTestObject(t, map[string]any{
		"a": 1,
		"b": 2,
		"c": 3,
	})

	count := 0
	keys := make(map[string]bool)

	for obj.Advance() {
		count++
		key, val := obj.CurrentKeyVal()
		keys[key.ToString()] = true

		s, ok := val.AsScalar()
		assertTrue(t, ok, "all values should be scalars")
		_ = s // Use the scalar
	}

	if count != 3 {
		t.Errorf("got %d pairs, want 3", count)
	}

	// Verify all keys were seen
	if !keys["a"] || !keys["b"] || !keys["c"] {
		t.Errorf("missing keys: got %v", keys)
	}

	// Done check: !obj.Advance(), "object should be done")
}

func TestObjectTypeAssertions(t *testing.T) {
	obj := makeTestObject(t, map[string]any{"key": "value"})

	// AsObject should succeed
	o, ok := obj.AsObject()
	assertTrue(t, ok, "AsObject should return true")
	assertNotNil(t, o, "AsObject should return non-nil object")

	// AsScalar should fail
	_, ok = obj.AsScalar()
	assertFalse(t, ok, "AsScalar should return false for object")

	// AsArray should fail
	_, ok = obj.AsArray()
	assertFalse(t, ok, "AsArray should return false for object")
}

func TestObjectCurrentKeyValPanicsWhenDone(t *testing.T) {
	obj := makeTestObject(t, map[string]any{"key": "value"})

	// Exhaust the object
	assertTrue(t, obj.Advance(), "should advance once")
	assertFalse(t, obj.Advance(), "should not advance again")

	// CurrentKeyVal should panic when done
	assertPanics(t, "iterator done", func() {
		obj.CurrentKeyVal()
	})
}

func TestObjectCloneBeforeAdvance(t *testing.T) {
	original := makeTestObject(t, map[string]any{
		"a": 1,
		"b": 2,
	})

	// Clone before advancing
	cloned, detach := original.CloneObject()
	defer detach()

	// Both should advance independently
	assertTrue(t, original.Advance(), "original should advance")
	assertTrue(t, cloned.Advance(), "clone should advance")

	// They should see the same data (order may vary)
	key1, _ := original.CurrentKeyVal()
	key2, _ := cloned.CurrentKeyVal()

	// Just verify both got valid keys
	if key1.ToString() == "" || key2.ToString() == "" {
		t.Error("both should have valid keys")
	}
}

func TestObjectCloneAfterAdvancePanics(t *testing.T) {
	obj := makeTestObject(t, map[string]any{"key": "value"})

	// Advance to start
	assertTrue(t, obj.Advance(), "should advance")

	// Attempting to clone should panic
	assertPanics(t, "cannot clone started collection", func() {
		obj.CloneObject()
	})
}

func TestObjectDiscard(t *testing.T) {
	obj := makeTestObject(t, map[string]any{
		"a": 1,
		"b": 2,
		"c": 3,
	})

	// Advance once
	assertTrue(t, obj.Advance(), "should advance to first")

	// Discard should skip remaining pairs
	obj.Discard()

	// Object should now be done
	// Done check: !obj.Advance(), "object should be done after discard")
	assertFalse(t, obj.Advance(), "should not advance after discard")
}

func TestObjectNestedObjects(t *testing.T) {
	obj := makeTestObject(t, map[string]any{
		"user": map[string]any{
			"name": "Alice",
			"age":  30,
		},
		"settings": map[string]any{
			"theme": "dark",
		},
	})

	foundUser := false
	foundSettings := false

	for obj.Advance() {
		key, val := obj.CurrentKeyVal()

		if key.ToString() == "user" {
			foundUser = true
			inner, ok := val.AsObject()
			assertTrue(t, ok, "user value should be object")

			// Verify inner object has data
			assertTrue(t, inner.Advance(), "inner object should have pairs")
		} else if key.ToString() == "settings" {
			foundSettings = true
			inner, ok := val.AsObject()
			assertTrue(t, ok, "settings value should be object")

			// Verify inner object has data
			assertTrue(t, inner.Advance(), "inner object should have pairs")
		}
	}

	assertTrue(t, foundUser, "should find user key")
	assertTrue(t, foundSettings, "should find settings key")
}

func TestObjectMixedValueTypes(t *testing.T) {
	obj := makeTestObject(t, map[string]any{
		"number": 42,
		"string": "hello",
		"bool":   true,
		"null":   nil,
		"array":  []any{1, 2, 3},
		"object": map[string]any{"nested": "value"},
	})

	count := 0
	for obj.Advance() {
		count++
		_, val := obj.CurrentKeyVal()
		assertNotNil(t, val, "value should not be nil")
	}

	if count != 6 {
		t.Errorf("got %d pairs, want 6", count)
	}
}

func TestObjectEqual(t *testing.T) {
	tests := []struct {
		name     string
		a, b     map[string]any
		expected bool
	}{
		{
			name:     "empty objects",
			a:        map[string]any{},
			b:        map[string]any{},
			expected: true,
		},
		{
			name:     "same single pair",
			a:        map[string]any{"key": "value"},
			b:        map[string]any{"key": "value"},
			expected: true,
		},
		{
			name:     "same multiple pairs same order",
			a:        map[string]any{"a": 1, "b": 2},
			b:        map[string]any{"a": 1, "b": 2},
			expected: true,
		},
		{
			name:     "different keys",
			a:        map[string]any{"a": 1},
			b:        map[string]any{"b": 1},
			expected: false,
		},
		{
			name:     "different values",
			a:        map[string]any{"key": 1},
			b:        map[string]any{"key": 2},
			expected: false,
		},
		{
			name:     "different number of pairs",
			a:        map[string]any{"a": 1},
			b:        map[string]any{"a": 1, "b": 2},
			expected: false,
		},
		{
			name: "nested objects same",
			a: map[string]any{
				"outer": map[string]any{"inner": 42},
			},
			b: map[string]any{
				"outer": map[string]any{"inner": 42},
			},
			expected: true,
		},
		{
			name: "nested objects different",
			a: map[string]any{
				"outer": map[string]any{"inner": 42},
			},
			b: map[string]any{
				"outer": map[string]any{"inner": 43},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj1 := makeTestObject(t, tt.a)
			obj2 := makeTestObject(t, tt.b)

			result := obj1.Equal(obj2)
			if result != tt.expected {
				t.Errorf("Equal() = %v, want %v", result, tt.expected)
			}

			// Equal should be symmetric - recreate objects since Equal() consumes them
			obj3 := makeTestObject(t, tt.b)
			obj4 := makeTestObject(t, tt.a)
			result2 := obj3.Equal(obj4)
			if result2 != tt.expected {
				t.Errorf("Equal() (reversed) = %v, want %v", result2, tt.expected)
			}
		})
	}
}

func TestObjectEqualDifferentTypes(t *testing.T) {
	obj := makeTestObject(t, map[string]any{"key": "value"})
	scalar := makeScalar(t, "test")
	arr := makeTestArray(t, 1, 2, 3)

	// Object vs Scalar
	assertFalse(t, obj.Equal(scalar), "object should not equal scalar")

	// Object vs Array
	assertFalse(t, obj.Equal(arr), "object should not equal array")
}

func TestObjectCopy(t *testing.T) {
	obj := makeTestObject(t, map[string]any{
		"a": 1,
		"b": 2,
	})

	// Copy to a channel
	tokens := make(chan token.Token, 20)
	done := make(chan bool)

	go func() {
		obj.Copy(token.ChannelWriteStream(tokens))
		close(tokens)
		done <- true
	}()

	// Collect tokens
	var collected []token.Token
	for tok := range tokens {
		collected = append(collected, tok)
	}
	<-done

	// Should have: StartObject, 2 keys, 2 values, EndObject = 6 tokens
	if len(collected) < 5 {
		t.Errorf("got %d tokens, want at least 5", len(collected))
	}

	// Verify structure
	_, ok := collected[0].(*token.StartObject)
	assertTrue(t, ok, "first token should be StartObject")

	_, ok = collected[len(collected)-1].(*token.EndObject)
	assertTrue(t, ok, "last token should be EndObject")
}
