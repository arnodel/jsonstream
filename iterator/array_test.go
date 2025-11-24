package iterator

import (
	"testing"

	"github.com/arnodel/jsonstream/token"
)

func TestArrayEmpty(t *testing.T) {
	arr := makeTestArray(t)

	// Empty array should not advance
	assertFalse(t, arr.Advance(), "empty array should not advance")
	// Done check: !arr.Advance(), "empty array should be done")
	assertFalse(t, arr.Elided(), "empty array should not be elided")
}

func TestArraySingleElement(t *testing.T) {
	arr := makeTestArray(t, 42)

	// Should advance once
	assertTrue(t, arr.Advance(), "should advance to first element")

	val := arr.CurrentValue()
	s, ok := val.AsScalar()
	assertTrue(t, ok, "element should be scalar")
	num := s.ToGo().(float64)
	if int64(num) != 42 {
		t.Errorf("got %d, want 42", int64(num))
	}

	// Should not advance again
	assertFalse(t, arr.Advance(), "should not advance past end")
	// Done check: !arr.Advance(), "should be done after exhaustion")
}

func TestArrayMultipleElements(t *testing.T) {
	arr := makeTestArray(t, 1, 2, 3, 4, 5)

	count := 0
	for arr.Advance() {
		count++
		val := arr.CurrentValue()
		s, ok := val.AsScalar()
		assertTrue(t, ok, "all elements should be scalars")
		num := s.ToGo().(float64)
		if int64(num) != int64(count) {
			t.Errorf("element %d: got %d, want %d", count, int64(num), count)
		}
	}

	if count != 5 {
		t.Errorf("got %d elements, want 5", count)
	}
	// Done check: !arr.Advance(), "array should be done")
}

func TestArrayTypeAssertions(t *testing.T) {
	arr := makeTestArray(t, 1, 2, 3)

	// AsArray should succeed
	a, ok := arr.AsArray()
	assertTrue(t, ok, "AsArray should return true")
	assertNotNil(t, a, "AsArray should return non-nil array")

	// AsScalar should fail
	_, ok = arr.AsScalar()
	assertFalse(t, ok, "AsScalar should return false for array")

	// AsObject should fail
	_, ok = arr.AsObject()
	assertFalse(t, ok, "AsObject should return false for array")
}

func TestArrayCurrentValuePanicsWhenDone(t *testing.T) {
	arr := makeTestArray(t, 1)

	// Exhaust the array
	assertTrue(t, arr.Advance(), "should advance once")
	assertFalse(t, arr.Advance(), "should not advance again")

	// CurrentValue should panic when done
	assertPanics(t, "iterator done", func() {
		arr.CurrentValue()
	})
}

func TestArrayCloneBeforeAdvance(t *testing.T) {
	original := makeTestArray(t, 1, 2, 3)

	// Clone before advancing
	cloned, detach := original.CloneArray()
	defer detach()

	// Both should advance independently
	assertTrue(t, original.Advance(), "original should advance")
	val1 := original.CurrentValue()
	s1, _ := val1.AsScalar()
	num1 := s1.ToGo().(float64)

	assertTrue(t, cloned.Advance(), "clone should advance")
	val2 := cloned.CurrentValue()
	s2, _ := val2.AsScalar()
	num2 := s2.ToGo().(float64)

	if int64(num1) != int64(num2) {
		t.Errorf("original and clone should have same first element")
	}
}

func TestArrayCloneAfterAdvancePanics(t *testing.T) {
	arr := makeTestArray(t, 1, 2, 3)

	// Advance to start
	assertTrue(t, arr.Advance(), "should advance")

	// Attempting to clone should panic
	assertPanics(t, "cannot clone started collection", func() {
		arr.CloneArray()
	})
}

func TestArrayDiscard(t *testing.T) {
	arr := makeTestArray(t, 1, 2, 3, 4, 5)

	// Advance once
	assertTrue(t, arr.Advance(), "should advance to first")

	// Discard should skip remaining elements
	arr.Discard()

	// Array should now be done
	// Done check: !arr.Advance(), "array should be done after discard")
	assertFalse(t, arr.Advance(), "should not advance after discard")
}

func TestArrayNestedArrays(t *testing.T) {
	arr := makeTestArray(t, []any{1, 2}, []any{3, 4}, []any{5, 6})

	// First element: array [1, 2]
	assertTrue(t, arr.Advance(), "should advance to first array")
	inner1, ok := arr.CurrentValue().AsArray()
	assertTrue(t, ok, "first element should be array")

	assertTrue(t, inner1.Advance(), "inner array should have first element")
	val := inner1.CurrentValue()
	s, _ := val.AsScalar()
	num := s.ToGo().(float64)
	if int64(num) != 1 {
		t.Errorf("got %d, want 1", int64(num))
	}

	// Don't consume rest of inner1, move to next
	assertTrue(t, arr.Advance(), "should advance to second array")
	inner2, ok := arr.CurrentValue().AsArray()
	assertTrue(t, ok, "second element should be array")

	assertTrue(t, inner2.Advance(), "inner array should have first element")
	val2 := inner2.CurrentValue()
	s2, _ := val2.AsScalar()
	num2 := s2.ToGo().(float64)
	if int64(num2) != 3 {
		t.Errorf("got %d, want 3", int64(num2))
	}
}

func TestArrayMixedTypes(t *testing.T) {
	arr := makeTestArray(t,
		42,
		"hello",
		true,
		nil,
		[]any{1, 2},
		map[string]any{"key": "value"},
	)

	// Int
	assertTrue(t, arr.Advance(), "should have element 1")
	_, ok := arr.CurrentValue().AsScalar()
	assertTrue(t, ok, "element 1 should be scalar")

	// String
	assertTrue(t, arr.Advance(), "should have element 2")
	_, ok = arr.CurrentValue().AsScalar()
	assertTrue(t, ok, "element 2 should be scalar")

	// Bool
	assertTrue(t, arr.Advance(), "should have element 3")
	_, ok = arr.CurrentValue().AsScalar()
	assertTrue(t, ok, "element 3 should be scalar")

	// Null
	assertTrue(t, arr.Advance(), "should have element 4")
	s, ok := arr.CurrentValue().AsScalar()
	assertTrue(t, ok, "element 4 should be scalar")
	if s.Type() != token.Null {
		t.Errorf("element 4 should be null")
	}

	// Array
	assertTrue(t, arr.Advance(), "should have element 5")
	_, ok = arr.CurrentValue().AsArray()
	assertTrue(t, ok, "element 5 should be array")

	// Object
	assertTrue(t, arr.Advance(), "should have element 6")
	_, ok = arr.CurrentValue().AsObject()
	assertTrue(t, ok, "element 6 should be object")

	// Done
	assertFalse(t, arr.Advance(), "should be done")
}

func TestArrayEqual(t *testing.T) {
	tests := []struct {
		name     string
		a, b     []any
		expected bool
	}{
		{
			name:     "empty arrays",
			a:        []any{},
			b:        []any{},
			expected: true,
		},
		{
			name:     "same single element",
			a:        []any{42},
			b:        []any{42},
			expected: true,
		},
		{
			name:     "same multiple elements",
			a:        []any{1, 2, 3},
			b:        []any{1, 2, 3},
			expected: true,
		},
		{
			name:     "different lengths",
			a:        []any{1, 2},
			b:        []any{1, 2, 3},
			expected: false,
		},
		{
			name:     "different elements",
			a:        []any{1, 2, 3},
			b:        []any{1, 2, 4},
			expected: false,
		},
		{
			name:     "nested arrays same",
			a:        []any{[]any{1, 2}, []any{3, 4}},
			b:        []any{[]any{1, 2}, []any{3, 4}},
			expected: true,
		},
		{
			name:     "nested arrays different",
			a:        []any{[]any{1, 2}, []any{3, 4}},
			b:        []any{[]any{1, 2}, []any{3, 5}},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arr1 := makeTestArray(t, tt.a...)
			arr2 := makeTestArray(t, tt.b...)

			result := arr1.Equal(arr2)
			if result != tt.expected {
				t.Errorf("Equal() = %v, want %v", result, tt.expected)
			}

			// Equal should be symmetric - recreate arrays since Equal() consumes them
			arr3 := makeTestArray(t, tt.b...)
			arr4 := makeTestArray(t, tt.a...)
			result2 := arr3.Equal(arr4)
			if result2 != tt.expected {
				t.Errorf("Equal() (reversed) = %v, want %v", result2, tt.expected)
			}
		})
	}
}

func TestArrayEqualDifferentTypes(t *testing.T) {
	arr := makeTestArray(t, 1, 2, 3)
	scalar := makeScalar(t, "test")
	obj := makeTestObject(t, map[string]any{"key": "value"})

	// Array vs Scalar
	assertFalse(t, arr.Equal(scalar), "array should not equal scalar")

	// Array vs Object
	assertFalse(t, arr.Equal(obj), "array should not equal object")
}

func TestArrayCopy(t *testing.T) {
	arr := makeTestArray(t, 1, 2, 3)

	// Copy to a channel
	tokens := make(chan token.Token, 20)
	done := make(chan bool)

	go func() {
		arr.Copy(token.ChannelWriteStream(tokens))
		close(tokens)
		done <- true
	}()

	// Collect tokens
	var collected []token.Token
	for tok := range tokens {
		collected = append(collected, tok)
	}
	<-done

	// Should have: StartArray, 3 scalars, EndArray = 5 tokens
	if len(collected) != 5 {
		t.Errorf("got %d tokens, want 5", len(collected))
	}

	// Verify structure
	_, ok := collected[0].(*token.StartArray)
	assertTrue(t, ok, "first token should be StartArray")

	_, ok = collected[4].(*token.EndArray)
	assertTrue(t, ok, "last token should be EndArray")
}
