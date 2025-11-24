package iterator

import (
	"testing"

	"github.com/arnodel/jsonstream/token"
)

// TestEdgeCaseEmptyIterator tests iterator with no values
func TestEdgeCaseEmptyIterator(t *testing.T) {
	it := makeIterator(t) // No values

	// Should not advance
	assertFalse(t, it.Advance(), "empty iterator should not advance")

	// CurrentValue should be nil
	assertNil(t, it.CurrentValue(), "empty iterator CurrentValue should be nil")

	// Multiple Advance calls should all return false
	assertFalse(t, it.Advance(), "second Advance should be false")
	assertFalse(t, it.Advance(), "third Advance should be false")
}

// TestEdgeCaseMultipleClonesSameValue tests cloning the same value multiple times
func TestEdgeCaseMultipleClonesSameValue(t *testing.T) {
	it := makeIterator(t, []any{1, 2, 3})
	assertTrue(t, it.Advance(), "should advance")

	arr, ok := it.CurrentValue().AsArray()
	assertTrue(t, ok, "should be array")

	// Clone multiple times before advancing
	clone1, detach1 := arr.CloneArray()
	defer detach1()

	clone2, detach2 := arr.CloneArray()
	defer detach2()

	clone3, detach3 := arr.CloneArray()
	defer detach3()

	// All clones should work independently
	for _, clone := range []*Array{arr, clone1, clone2, clone3} {
		count := 0
		for clone.Advance() {
			count++
		}
		if count != 3 {
			t.Errorf("expected 3 elements, got %d", count)
		}
	}
}

// TestEdgeCaseCloneAfterPartialConsumption tests cloning after consuming some elements
func TestEdgeCaseCloneAfterPartialConsumption(t *testing.T) {
	it := makeIterator(t, 1, 2, 3, 4, 5)

	// Clone before any advance
	clone, detach := it.Clone()
	defer detach()

	// Advance original by 2
	assertTrue(t, it.Advance(), "should advance")
	assertTrue(t, it.Advance(), "should advance")

	// Original is now at position 2
	val := it.CurrentValue()
	s, _ := val.AsScalar()
	num := s.ToGo().(float64)
	if int64(num) != 2 {
		t.Errorf("expected 2, got %d", int64(num))
	}

	// Clone should start from beginning
	assertTrue(t, clone.Advance(), "clone should advance")
	val2 := clone.CurrentValue()
	s2, _ := val2.AsScalar()
	num2 := s2.ToGo().(float64)
	if int64(num2) != 1 {
		t.Errorf("expected clone to start at 1, got %d", int64(num2))
	}
}

// TestEdgeCaseDiscardWithoutAdvance tests discarding before any advance
func TestEdgeCaseDiscardWithoutAdvance(t *testing.T) {
	it := makeIterator(t, []any{1, 2, 3})
	assertTrue(t, it.Advance(), "should advance")

	arr, ok := it.CurrentValue().AsArray()
	assertTrue(t, ok, "should be array")

	// Discard without advancing - should skip entire array
	arr.Discard()

	// Should not be able to advance
	assertFalse(t, arr.Advance(), "should not advance after discard")
}

// TestEdgeCaseDiscardAfterSomeAdvances tests discarding partway through
func TestEdgeCaseDiscardAfterSomeAdvances(t *testing.T) {
	it := makeIterator(t, []any{1, 2, 3, 4, 5})
	assertTrue(t, it.Advance(), "should advance")

	arr, ok := it.CurrentValue().AsArray()
	assertTrue(t, ok, "should be array")

	// Advance twice
	assertTrue(t, arr.Advance(), "should advance")
	assertTrue(t, arr.Advance(), "should advance")

	// Discard - should skip remaining elements
	arr.Discard()

	// Should not advance
	assertFalse(t, arr.Advance(), "should not advance after discard")
}

// TestEdgeCaseEqualWithSelf tests value equality with itself
func TestEdgeCaseEqualWithSelf(t *testing.T) {
	scalar := makeScalar(t, 42)
	arr := makeTestArray(t, 1, 2, 3)
	obj := makeTestObject(t, map[string]any{"key": "value"})

	// Each value should equal itself (before consumption)
	// Note: we need fresh copies since Equal() consumes the values

	scalar1 := makeScalar(t, 42)
	scalar2 := makeScalar(t, 42)
	assertTrue(t, scalar1.Equal(scalar2), "scalar should equal itself")

	arr1 := makeTestArray(t, 1, 2, 3)
	arr2 := makeTestArray(t, 1, 2, 3)
	assertTrue(t, arr1.Equal(arr2), "array should equal itself")

	obj1 := makeTestObject(t, map[string]any{"key": "value"})
	obj2 := makeTestObject(t, map[string]any{"key": "value"})
	assertTrue(t, obj1.Equal(obj2), "object should equal itself")

	// Verify that we can't use the original values after equality
	// (they've been consumed)
	_ = scalar
	_ = arr
	_ = obj
}

// TestEdgeCaseCurrentValueBeforeAdvance tests CurrentValue before any Advance
func TestEdgeCaseCurrentValueBeforeAdvance(t *testing.T) {
	it := makeIterator(t, 1, 2, 3)

	// Before Advance, CurrentValue should be nil
	val := it.CurrentValue()
	assertNil(t, val, "CurrentValue should be nil before Advance")

	// After Advance, should have value
	assertTrue(t, it.Advance(), "should advance")
	val = it.CurrentValue()
	assertNotNil(t, val, "CurrentValue should not be nil after Advance")
}

// TestEdgeCaseCurrentValueAfterExhaustion tests CurrentValue after iterator is done
func TestEdgeCaseCurrentValueAfterExhaustion(t *testing.T) {
	it := makeIterator(t, 1, 2)

	assertTrue(t, it.Advance(), "should advance")
	assertTrue(t, it.Advance(), "should advance")
	assertFalse(t, it.Advance(), "should not advance")

	// After exhaustion, CurrentValue should be nil
	val := it.CurrentValue()
	assertNil(t, val, "CurrentValue should be nil after exhaustion")
}

// TestEdgeCaseCurrentKeyValOnExhaustedObject tests CurrentKeyVal on exhausted object
func TestEdgeCaseCurrentKeyValOnExhaustedObject(t *testing.T) {
	obj := makeTestObject(t, map[string]any{"key": "value"})

	// Exhaust the object
	assertTrue(t, obj.Advance(), "should advance")
	assertFalse(t, obj.Advance(), "should not advance")

	// CurrentKeyVal should panic when done
	assertPanics(t, "iterator done", func() {
		obj.CurrentKeyVal()
	})
}

// TestEdgeCaseElisionInArray tests array with elision markers
func TestEdgeCaseElisionInArray(t *testing.T) {
	// Create an array with elision: [1, ..., 3]
	tokens := []token.Token{
		&token.StartArray{},
		token.Int64Scalar(1),
		&token.Elision{},
		token.Int64Scalar(3),
		&token.EndArray{},
	}

	stream := token.NewSliceReadStream(tokens)
	it := New(stream)

	assertTrue(t, it.Advance(), "should advance")
	arr, ok := it.CurrentValue().AsArray()
	assertTrue(t, ok, "should be array")

	// First element
	assertTrue(t, arr.Advance(), "should advance to first element")
	s1, _ := arr.CurrentValue().AsScalar()
	num1 := s1.ToGo().(float64)
	if int64(num1) != 1 {
		t.Errorf("expected 1, got %d", int64(num1))
	}

	// Elided marker
	assertTrue(t, arr.Advance(), "should advance to elision")
	assertTrue(t, arr.Elided(), "should be elided")

	// After elision, should be done
	assertFalse(t, arr.Advance(), "should not advance after elision")
}

// TestEdgeCaseElisionInObject tests object with elision markers
func TestEdgeCaseElisionInObject(t *testing.T) {
	// Create an object with elision: {"key1": "value1", ...}
	tokens := []token.Token{
		&token.StartObject{},
		token.StringScalar("key1"),
		token.StringScalar("value1"),
		&token.Elision{},
		&token.EndObject{},
	}

	stream := token.NewSliceReadStream(tokens)
	it := New(stream)

	assertTrue(t, it.Advance(), "should advance")
	obj, ok := it.CurrentValue().AsObject()
	assertTrue(t, ok, "should be object")

	// First pair
	assertTrue(t, obj.Advance(), "should advance to first pair")
	key, val := obj.CurrentKeyVal()
	if key.ToString() != "key1" {
		t.Errorf("expected key1, got %q", key.ToString())
	}
	s, _ := val.AsScalar()
	if s.ToString() != "value1" {
		t.Errorf("expected value1, got %q", s.ToString())
	}

	// Next advance should hit elision and return false (done)
	assertFalse(t, obj.Advance(), "should not advance after elision")

	// After processing elision, Elided() should return true
	assertTrue(t, obj.Elided(), "should be elided")
}

// TestEdgeCaseDeepNesting tests deeply nested structures
func TestEdgeCaseDeepNesting(t *testing.T) {
	// Create deeply nested array: [[[[1]]]]
	depth := 10
	var buildNested func(int) any
	buildNested = func(d int) any {
		if d == 0 {
			return 1
		}
		return []any{buildNested(d - 1)}
	}

	nested := buildNested(depth)

	it := makeIterator(t, nested)
	assertTrue(t, it.Advance(), "should advance")

	// Navigate down through all the nesting
	current := it.CurrentValue()
	for i := 0; i < depth; i++ {
		arr, ok := current.AsArray()
		if !ok {
			t.Fatalf("should be array at depth %d", i)
		}
		if !arr.Advance() {
			t.Fatalf("should advance at depth %d", i)
		}
		current = arr.CurrentValue()
	}

	// Should finally reach the scalar
	s, ok := current.AsScalar()
	assertTrue(t, ok, "should reach scalar")
	num := s.ToGo().(float64)
	if int64(num) != 1 {
		t.Errorf("expected 1, got %d", int64(num))
	}
}

// TestEdgeCaseVeryLongStrings tests scalars with very long string values
func TestEdgeCaseVeryLongStrings(t *testing.T) {
	// Create a very long string
	longStr := ""
	for i := 0; i < 1000; i++ {
		longStr += "abcdefghij"
	}

	it := makeIterator(t, longStr)
	assertTrue(t, it.Advance(), "should advance")

	scalar, ok := it.CurrentValue().AsScalar()
	assertTrue(t, ok, "should be scalar")

	result := scalar.ToString()
	if len(result) != len(longStr) {
		t.Errorf("expected length %d, got %d", len(longStr), len(result))
	}
	if result != longStr {
		t.Error("string mismatch")
	}
}

// TestEdgeCaseManySmallValues tests iterator with many small values
func TestEdgeCaseManySmallValues(t *testing.T) {
	count := 1000
	values := make([]any, count)
	for i := 0; i < count; i++ {
		values[i] = i
	}

	it := makeIterator(t, values...)

	// Process all values
	processed := 0
	for it.Advance() {
		val := it.CurrentValue()
		s, ok := val.AsScalar()
		assertTrue(t, ok, "should be scalar")
		num := s.ToGo().(float64)
		if int64(num) != int64(processed) {
			t.Errorf("value %d: expected %d, got %d", processed, processed, int64(num))
		}
		processed++
	}

	if processed != count {
		t.Errorf("expected %d values, processed %d", count, processed)
	}
}

// TestEdgeCaseObjectWithDuplicateKeys tests object with duplicate keys (last wins)
func TestEdgeCaseObjectWithDuplicateKeys(t *testing.T) {
	// Manually construct tokens with duplicate keys
	tokens := []token.Token{
		&token.StartObject{},
		token.StringScalar("key"),
		token.StringScalar("value1"),
		token.StringScalar("key"),
		token.StringScalar("value2"),
		&token.EndObject{},
	}

	stream := token.NewSliceReadStream(tokens)
	it := New(stream)

	assertTrue(t, it.Advance(), "should advance")
	obj, ok := it.CurrentValue().AsObject()
	assertTrue(t, ok, "should be object")

	// Should see both key-value pairs
	count := 0
	for obj.Advance() {
		count++
		key, val := obj.CurrentKeyVal()
		if key.ToString() != "key" {
			t.Errorf("expected key 'key', got %q", key.ToString())
		}
		s, _ := val.AsScalar()
		// Should see both value1 and value2
		valStr := s.ToString()
		if valStr != "value1" && valStr != "value2" {
			t.Errorf("unexpected value: %q", valStr)
		}
	}

	if count != 2 {
		t.Errorf("expected 2 pairs (including duplicate), got %d", count)
	}
}

// TestEdgeCaseEqualWithEmptyVsNonEmpty tests equality between empty and non-empty
func TestEdgeCaseEqualWithEmptyVsNonEmpty(t *testing.T) {
	emptyArr := makeTestArray(t)
	nonEmptyArr := makeTestArray(t, 1)

	assertFalse(t, emptyArr.Equal(nonEmptyArr), "empty should not equal non-empty")

	emptyArr2 := makeTestArray(t)
	nonEmptyArr2 := makeTestArray(t, 1)
	assertFalse(t, nonEmptyArr2.Equal(emptyArr2), "non-empty should not equal empty")
}

// TestEdgeCaseCopyEmptyCollection tests copying empty collections
func TestEdgeCaseCopyEmptyCollection(t *testing.T) {
	emptyArr := makeTestArray(t)

	tokens := make(chan token.Token, 10)
	out := token.ChannelWriteStream(tokens)

	go func() {
		emptyArr.Copy(out)
		close(tokens)
	}()

	// Collect tokens
	var collected []token.Token
	for tok := range tokens {
		collected = append(collected, tok)
	}

	// Should have: StartArray, EndArray
	if len(collected) != 2 {
		t.Errorf("expected 2 tokens, got %d", len(collected))
	}

	_, ok := collected[0].(*token.StartArray)
	assertTrue(t, ok, "first token should be StartArray")

	_, ok = collected[1].(*token.EndArray)
	assertTrue(t, ok, "second token should be EndArray")
}

// TestEdgeCaseScalarTypeCornerCases tests edge cases for different scalar types
func TestEdgeCaseScalarTypeCornerCases(t *testing.T) {
	tests := []struct {
		name  string
		value any
		check func(*testing.T, *token.Scalar)
	}{
		{
			name:  "zero",
			value: 0,
			check: func(t *testing.T, s *token.Scalar) {
				num := s.ToGo().(float64)
				if num != 0 {
					t.Errorf("expected 0, got %v", num)
				}
			},
		},
		{
			name:  "negative number",
			value: -42,
			check: func(t *testing.T, s *token.Scalar) {
				num := s.ToGo().(float64)
				if int64(num) != -42 {
					t.Errorf("expected -42, got %v", int64(num))
				}
			},
		},
		{
			name:  "empty string",
			value: "",
			check: func(t *testing.T, s *token.Scalar) {
				str := s.ToString()
				if str != "" {
					t.Errorf("expected empty string, got %q", str)
				}
			},
		},
		{
			name:  "string with special chars",
			value: "hello\nworld\t!",
			check: func(t *testing.T, s *token.Scalar) {
				str := s.ToString()
				if str != "hello\nworld\t!" {
					t.Errorf("expected special chars, got %q", str)
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

// TestEdgeCaseTransformerWithNoOutput tests transformer that produces no output
func TestEdgeCaseTransformerWithNoOutput(t *testing.T) {
	// countingTransformer produces no output
	transformer := &countingTransformer{}

	it := makeIterator(t, 1, 2, 3, 4, 5)

	tokens := make(chan token.Token, 10)
	out := token.ChannelWriteStream(tokens)

	for it.Advance() {
		transformer.TransformValue(it.CurrentValue(), out)
	}
	close(tokens)

	// Should have no tokens
	var collected []token.Token
	for tok := range tokens {
		collected = append(collected, tok)
	}

	if len(collected) != 0 {
		t.Errorf("expected 0 tokens, got %d", len(collected))
	}

	// But should have counted
	if transformer.count != 5 {
		t.Errorf("expected count=5, got %d", transformer.count)
	}
}

// TestEdgeCaseIteratorFromEmptyStream tests creating iterator from empty stream
func TestEdgeCaseIteratorFromEmptyStream(t *testing.T) {
	tokens := []token.Token{} // Empty
	stream := token.NewSliceReadStream(tokens)
	it := New(stream)

	assertFalse(t, it.Advance(), "should not advance on empty stream")
	assertNil(t, it.CurrentValue(), "should have nil value")
}

// TestEdgeCaseMultipleDetachCalls tests calling detach multiple times
func TestEdgeCaseMultipleDetachCalls(t *testing.T) {
	it := makeIterator(t, []any{1, 2, 3})

	// Clone
	_, detach := it.Clone()

	// Call detach multiple times - should not panic
	detach()
	detach()
	detach()
}

// TestEdgeCaseCloneScalarDetach tests that scalar clone detach is nil
func TestEdgeCaseCloneScalarDetach(t *testing.T) {
	scalar := makeScalar(t, 42)

	cloned, detach := scalar.Clone()

	// Detach should be nil for immutable scalar
	if detach != nil {
		t.Error("scalar detach should be nil")
	}

	// Cloned should be the same instance
	if cloned != scalar {
		t.Error("scalar clone should return same instance")
	}
}

// TestEdgeCaseArrayEqualAfterModification tests that equality works correctly after partial consumption
func TestEdgeCaseArrayEqualAfterModification(t *testing.T) {
	// Create two identical arrays
	arr1 := makeTestArray(t, 1, 2, 3, 4, 5)
	arr2 := makeTestArray(t, 1, 2, 3, 4, 5)

	// Partially consume arr1
	arr1.Advance()
	arr1.Advance()
	// arr1 now has [3, 4, 5] remaining

	// arr2 is fresh with [1, 2, 3, 4, 5]

	// They should NOT be equal (different positions)
	result := arr1.Equal(arr2)
	assertFalse(t, result, "partially consumed array should not equal fresh array")
}
