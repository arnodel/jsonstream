package iterator

import (
	"testing"

	"github.com/arnodel/jsonstream/token"
)

func TestIteratorBasicAdvance(t *testing.T) {
	it := makeIterator(t, 1, 2, 3)

	// Before advance, CurrentValue should be nil
	assertNil(t, it.CurrentValue(), "CurrentValue should be nil before first advance")

	// First advance
	assertTrue(t, it.Advance(), "first Advance should succeed")
	val1 := it.CurrentValue()
	assertNotNil(t, val1, "CurrentValue should not be nil after advance")
	s1, ok := val1.AsScalar()
	assertTrue(t, ok, "first value should be scalar")
	i1 := s1.ToGo().(float64)
	assertEqual(t, int64(i1), int64(1))

	// Second advance
	assertTrue(t, it.Advance(), "second Advance should succeed")
	val2 := it.CurrentValue()
	s2, _ := val2.AsScalar()
	i2 := s2.ToGo().(float64)
	assertEqual(t, int64(i2), int64(2))

	// Third advance
	assertTrue(t, it.Advance(), "third Advance should succeed")
	val3 := it.CurrentValue()
	s3, _ := val3.AsScalar()
	i3 := s3.ToGo().(float64)
	assertEqual(t, int64(i3), int64(3))

	// Fourth advance should fail (no more values)
	assertFalse(t, it.Advance(), "fourth Advance should fail")

	// CurrentValue after exhaustion
	assertNil(t, it.CurrentValue(), "CurrentValue should be nil after exhaustion")
}

func TestIteratorEmpty(t *testing.T) {
	it := makeIterator(t)

	// Empty iterator should immediately return false
	assertFalse(t, it.Advance(), "Advance on empty iterator should return false")
	assertNil(t, it.CurrentValue(), "CurrentValue should be nil for empty iterator")
}

func TestIteratorCloneBeforeAdvance(t *testing.T) {
	original := makeIterator(t, 1, 2, 3)

	// Clone before advancing
	cloned, detach := original.Clone()
	defer detach()

	// Both iterators should work independently
	assertTrue(t, original.Advance(), "original Advance should succeed")
	val1 := original.CurrentValue()
	s1, _ := val1.AsScalar()
	i1 := s1.ToGo().(float64)
	assertEqual(t, int64(i1), int64(1))

	assertTrue(t, cloned.Advance(), "cloned Advance should succeed")
	val2 := cloned.CurrentValue()
	s2, _ := val2.AsScalar()
	i2 := s2.ToGo().(float64)
	assertEqual(t, int64(i2), int64(1))

	// Continue with original
	assertTrue(t, original.Advance(), "original second Advance should succeed")
	val3 := original.CurrentValue()
	s3, _ := val3.AsScalar()
	i3 := s3.ToGo().(float64)
	assertEqual(t, int64(i3), int64(2))

	// Cloned should still be at its position
	assertTrue(t, cloned.Advance(), "cloned second Advance should succeed")
	val4 := cloned.CurrentValue()
	s4, _ := val4.AsScalar()
	i4 := s4.ToGo().(float64)
	assertEqual(t, int64(i4), int64(2))
}

func TestIteratorCloneAfterAdvancePanics(t *testing.T) {
	it := makeIterator(t, 1, 2, 3)

	// Advance to start the iterator
	assertTrue(t, it.Advance(), "first Advance should succeed")

	// Attempting to clone should panic
	assertPanics(t, "cannot clone started iterator", func() {
		it.Clone()
	})
}

func TestIteratorMultipleClones(t *testing.T) {
	original := makeIterator(t, 1, 2, 3)

	// Clone twice
	clone1, detach1 := original.Clone()
	defer detach1()

	clone2, detach2 := original.Clone()
	defer detach2()

	// All three should work independently
	assertTrue(t, original.Advance(), "original should advance")
	assertTrue(t, clone1.Advance(), "clone1 should advance")
	assertTrue(t, clone2.Advance(), "clone2 should advance")

	// All should have same first value
	v1 := original.CurrentValue()
	v2 := clone1.CurrentValue()
	v3 := clone2.CurrentValue()

	assertTrue(t, v1.Equal(v2), "original and clone1 should have equal values")
	assertTrue(t, v2.Equal(v3), "clone1 and clone2 should have equal values")
}

func TestIteratorAutoDiscard(t *testing.T) {
	// Create an iterator with arrays (collections that need discarding)
	it := makeIterator(t, []any{1, 2}, []any{3, 4})

	// Advance to first array
	assertTrue(t, it.Advance(), "first Advance should succeed")
	_, ok := it.CurrentValue().AsArray()
	assertTrue(t, ok, "first value should be array")

	// Don't consume the array, just advance to next
	// This should auto-discard the first array
	assertTrue(t, it.Advance(), "second Advance should succeed")
	arr2, ok := it.CurrentValue().AsArray()
	assertTrue(t, ok, "second value should be array")

	// Verify we got the second array by consuming it
	assertTrue(t, arr2.Advance(), "array should have first element")
	val := arr2.CurrentValue()
	s, _ := val.AsScalar()
	i := s.ToGo().(float64)
	assertEqual(t, int64(i), int64(3))
}

func TestIteratorWithMixedTypes(t *testing.T) {
	// Mix of scalars, arrays, and objects
	it := makeIterator(t,
		"string",
		42,
		[]any{1, 2, 3},
		map[string]any{"key": "value"},
		nil,
	)

	// String
	assertTrue(t, it.Advance(), "should advance to string")
	_, ok := it.CurrentValue().AsScalar()
	assertTrue(t, ok, "first should be scalar")

	// Int
	assertTrue(t, it.Advance(), "should advance to int")
	_, ok = it.CurrentValue().AsScalar()
	assertTrue(t, ok, "second should be scalar")

	// Array
	assertTrue(t, it.Advance(), "should advance to array")
	_, ok = it.CurrentValue().AsArray()
	assertTrue(t, ok, "third should be array")

	// Object
	assertTrue(t, it.Advance(), "should advance to object")
	_, ok = it.CurrentValue().AsObject()
	assertTrue(t, ok, "fourth should be object")

	// Null
	assertTrue(t, it.Advance(), "should advance to null")
	s, ok := it.CurrentValue().AsScalar()
	assertTrue(t, ok, "fifth should be scalar")
	if s.Type() != token.Null {
		t.Errorf("expected null type (%d), got %d", token.Null, s.Type())
	}

	// End
	assertFalse(t, it.Advance(), "should be exhausted")
}

func TestIteratorFromArray(t *testing.T) {
	// Test iterating through a nested array structure
	it := makeIterator(t, []any{
		1,
		"hello",
		true,
		nil,
		map[string]any{"key": "value"},
		[]any{1, 2, 3},
	})

	// Advance to the array
	assertTrue(t, it.Advance(), "should advance to array")
	arr, ok := it.CurrentValue().AsArray()
	assertTrue(t, ok, "should be array")

	// Iterate through array elements
	assertTrue(t, arr.Advance(), "should have element 1")
	assertTrue(t, arr.Advance(), "should have element 2")
	assertTrue(t, arr.Advance(), "should have element 3")
	assertTrue(t, arr.Advance(), "should have element 4")
	assertTrue(t, arr.Advance(), "should have element 5")
	assertTrue(t, arr.Advance(), "should have element 6")
	assertFalse(t, arr.Advance(), "should have no more elements")
}

func TestIteratorCollectValues(t *testing.T) {
	it := makeIterator(t, 1, 2, 3, 4, 5)
	values := collectValues(t, it)

	assertEqual(t, len(values), 5)

	for i, val := range values {
		s, ok := val.AsScalar()
		assertTrue(t, ok, "all values should be scalars")
		num := s.ToGo().(float64)
		assertEqual(t, int64(num), int64(i+1))
	}
}
