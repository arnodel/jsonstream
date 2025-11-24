package iterator

import (
	"testing"
)

// TestEqualityNilValues tests Equal behavior with nil values
func TestEqualityNilValues(t *testing.T) {
	scalar := makeScalar(t, "test")
	arr := makeTestArray(t, 1, 2, 3)
	obj := makeTestObject(t, map[string]any{"key": "value"})

	// All value types should handle nil gracefully
	assertFalse(t, scalar.Equal(nil), "scalar.Equal(nil) should be false")
	assertFalse(t, arr.Equal(nil), "array.Equal(nil) should be false")
	assertFalse(t, obj.Equal(nil), "object.Equal(nil) should be false")
}

// TestEqualityCrossTypeComparisons tests equality across different value types
func TestEqualityCrossTypeComparisons(t *testing.T) {
	scalar := makeScalar(t, 42)
	arr := makeTestArray(t, 42)
	obj := makeTestObject(t, map[string]any{"value": 42})

	// Scalar vs Array
	assertFalse(t, scalar.Equal(arr), "scalar should not equal array")
	assertFalse(t, arr.Equal(scalar), "array should not equal scalar")

	// Scalar vs Object
	assertFalse(t, scalar.Equal(obj), "scalar should not equal object")
	assertFalse(t, obj.Equal(scalar), "object should not equal scalar")

	// Array vs Object
	assertFalse(t, arr.Equal(obj), "array should not equal object")
	assertFalse(t, obj.Equal(arr), "object should not equal array")
}

// TestEqualityEmptyCollections tests equality of empty arrays and objects
func TestEqualityEmptyCollections(t *testing.T) {
	emptyArr1 := makeTestArray(t)
	emptyArr2 := makeTestArray(t)
	emptyObj1 := makeTestObject(t, map[string]any{})
	emptyObj2 := makeTestObject(t, map[string]any{})

	// Empty arrays should be equal
	assertTrue(t, emptyArr1.Equal(emptyArr2), "empty arrays should be equal")

	// Empty objects should be equal
	assertTrue(t, emptyObj1.Equal(emptyObj2), "empty objects should be equal")

	// Empty array should not equal empty object
	assertFalse(t, emptyArr1.Equal(emptyObj1), "empty array should not equal empty object")
	assertFalse(t, emptyObj1.Equal(emptyArr1), "empty object should not equal empty array")
}

// TestEqualityDeeplyNestedStructures tests equality with deeply nested structures
func TestEqualityDeeplyNestedStructures(t *testing.T) {
	// Create deeply nested identical structures
	nested1 := makeTestArray(t,
		map[string]any{
			"level1": map[string]any{
				"level2": []any{
					map[string]any{
						"level3": []any{1, 2, 3},
					},
				},
			},
		},
	)

	nested2 := makeTestArray(t,
		map[string]any{
			"level1": map[string]any{
				"level2": []any{
					map[string]any{
						"level3": []any{1, 2, 3},
					},
				},
			},
		},
	)

	assertTrue(t, nested1.Equal(nested2), "deeply nested identical structures should be equal")

	// Create similar structure with difference deep inside
	nested3 := makeTestArray(t,
		map[string]any{
			"level1": map[string]any{
				"level2": []any{
					map[string]any{
						"level3": []any{1, 2, 4}, // Different value
					},
				},
			},
		},
	)

	nested4 := makeTestArray(t,
		map[string]any{
			"level1": map[string]any{
				"level2": []any{
					map[string]any{
						"level3": []any{1, 2, 3},
					},
				},
			},
		},
	)

	assertFalse(t, nested3.Equal(nested4), "structures with deep differences should not be equal")
}

// TestEqualityArraysWithDifferentElementTypes tests arrays containing different types
func TestEqualityArraysWithDifferentElementTypes(t *testing.T) {
	tests := []struct {
		name     string
		a, b     []any
		expected bool
	}{
		{
			name:     "int vs string at same position",
			a:        []any{42},
			b:        []any{"42"},
			expected: false,
		},
		{
			name:     "true vs 1",
			a:        []any{true},
			b:        []any{1},
			expected: false,
		},
		{
			name:     "null vs 0",
			a:        []any{nil},
			b:        []any{0},
			expected: false,
		},
		{
			name:     "null vs empty string",
			a:        []any{nil},
			b:        []any{""},
			expected: false,
		},
		{
			name:     "empty array vs empty object",
			a:        []any{[]any{}},
			b:        []any{map[string]any{}},
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
		})
	}
}

// TestEqualityObjectsWithSpecialKeys tests objects with various key patterns
func TestEqualityObjectsWithSpecialKeys(t *testing.T) {
	tests := []struct {
		name     string
		a, b     map[string]any
		expected bool
	}{
		{
			name:     "empty string key",
			a:        map[string]any{"": "value"},
			b:        map[string]any{"": "value"},
			expected: true,
		},
		{
			name:     "unicode keys",
			a:        map[string]any{"日本語": "value"},
			b:        map[string]any{"日本語": "value"},
			expected: true,
		},
		{
			name:     "keys with spaces",
			a:        map[string]any{"key with spaces": "value"},
			b:        map[string]any{"key with spaces": "value"},
			expected: true,
		},
		{
			name:     "keys with special characters",
			a:        map[string]any{"key.with.dots": "value"},
			b:        map[string]any{"key.with.dots": "value"},
			expected: true,
		},
		{
			name:     "similar but different keys",
			a:        map[string]any{"key": "value"},
			b:        map[string]any{"Key": "value"},
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
		})
	}
}

// TestEqualityLargeArrays tests equality with larger arrays
func TestEqualityLargeArrays(t *testing.T) {
	// Create two large identical arrays
	size := 100
	items1 := make([]any, size)
	items2 := make([]any, size)
	for i := 0; i < size; i++ {
		items1[i] = i
		items2[i] = i
	}

	arr1 := makeTestArray(t, items1...)
	arr2 := makeTestArray(t, items2...)

	assertTrue(t, arr1.Equal(arr2), "large identical arrays should be equal")

	// Create array with difference at the end
	items3 := make([]any, size)
	copy(items3, items1)
	items3[size-1] = size // Different from size-1

	arr3 := makeTestArray(t, items3...)
	arr4 := makeTestArray(t, items1...)

	assertFalse(t, arr3.Equal(arr4), "arrays with difference at end should not be equal")
}

// TestEqualityLargeObjects tests equality with larger objects
func TestEqualityLargeObjects(t *testing.T) {
	// Create two large identical objects
	size := 50
	pairs1 := make(map[string]any, size)
	pairs2 := make(map[string]any, size)
	for i := 0; i < size; i++ {
		key := string(rune('a' + i%26))
		if i >= 26 {
			key = key + string(rune('0' + i/26))
		}
		pairs1[key] = i
		pairs2[key] = i
	}

	obj1 := makeTestObject(t, pairs1)
	obj2 := makeTestObject(t, pairs2)

	assertTrue(t, obj1.Equal(obj2), "large identical objects should be equal")
}

// TestEqualityMixedComplexStructures tests complex mixed structures
func TestEqualityMixedComplexStructures(t *testing.T) {
	complex1 := makeTestObject(t, map[string]any{
		"users": []any{
			map[string]any{"name": "Alice", "age": 30, "active": true},
			map[string]any{"name": "Bob", "age": 25, "active": false},
		},
		"metadata": map[string]any{
			"count":   2,
			"updated": "2024-01-01",
		},
		"tags": []any{"admin", "user", "guest"},
	})

	complex2 := makeTestObject(t, map[string]any{
		"users": []any{
			map[string]any{"name": "Alice", "age": 30, "active": true},
			map[string]any{"name": "Bob", "age": 25, "active": false},
		},
		"metadata": map[string]any{
			"count":   2,
			"updated": "2024-01-01",
		},
		"tags": []any{"admin", "user", "guest"},
	})

	assertTrue(t, complex1.Equal(complex2), "complex identical structures should be equal")

	// Change one nested value
	complex3 := makeTestObject(t, map[string]any{
		"users": []any{
			map[string]any{"name": "Alice", "age": 31, "active": true}, // Age changed
			map[string]any{"name": "Bob", "age": 25, "active": false},
		},
		"metadata": map[string]any{
			"count":   2,
			"updated": "2024-01-01",
		},
		"tags": []any{"admin", "user", "guest"},
	})

	complex4 := makeTestObject(t, map[string]any{
		"users": []any{
			map[string]any{"name": "Alice", "age": 30, "active": true},
			map[string]any{"name": "Bob", "age": 25, "active": false},
		},
		"metadata": map[string]any{
			"count":   2,
			"updated": "2024-01-01",
		},
		"tags": []any{"admin", "user", "guest"},
	})

	assertFalse(t, complex3.Equal(complex4), "complex structures with subtle differences should not be equal")
}

// TestEqualityAfterPartialConsumption tests equality when values are partially consumed
func TestEqualityAfterPartialConsumption(t *testing.T) {
	// Create two identical arrays
	arr1 := makeTestArray(t, 1, 2, 3, 4, 5)
	arr2 := makeTestArray(t, 1, 2, 3, 4, 5)

	// Partially consume arr1
	arr1.Advance()
	arr1.Advance()

	// Now arr1 is at position 2, arr2 is at beginning
	// They should not be equal (different positions/remaining content)
	result := arr1.Equal(arr2)

	// The Equal implementation will compare from current positions
	// arr1 has [3,4,5] remaining, arr2 has [1,2,3,4,5]
	assertFalse(t, result, "partially consumed array should not equal fresh array")
}
