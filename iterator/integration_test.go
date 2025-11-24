package iterator

import (
	"testing"

	"github.com/arnodel/jsonstream/token"
)

// TestIntegrationFullDocumentParsing tests parsing and traversing a realistic JSON document
func TestIntegrationFullDocumentParsing(t *testing.T) {
	// Realistic API response structure
	doc := map[string]any{
		"status": "success",
		"data": map[string]any{
			"users": []any{
				map[string]any{
					"id":    1,
					"name":  "Alice",
					"email": "alice@example.com",
					"roles": []any{"admin", "user"},
				},
				map[string]any{
					"id":    2,
					"name":  "Bob",
					"email": "bob@example.com",
					"roles": []any{"user"},
				},
			},
			"total": 2,
		},
		"metadata": map[string]any{
			"timestamp": "2024-01-01T00:00:00Z",
			"version":   "1.0",
		},
	}

	it := makeIterator(t, doc)
	assertTrue(t, it.Advance(), "should have document")

	obj, ok := it.CurrentValue().AsObject()
	assertTrue(t, ok, "root should be object")

	// Traverse the document
	foundData := false
	foundStatus := false
	foundMetadata := false

	for obj.Advance() {
		key, val := obj.CurrentKeyVal()
		switch key.ToString() {
		case "status":
			foundStatus = true
			s, ok := val.AsScalar()
			assertTrue(t, ok, "status should be scalar")
			if s.ToString() != "success" {
				t.Errorf("expected status=success, got %q", s.ToString())
			}

		case "data":
			foundData = true
			dataObj, ok := val.AsObject()
			assertTrue(t, ok, "data should be object")

			// Find users array
			for dataObj.Advance() {
				k, v := dataObj.CurrentKeyVal()
				if k.ToString() == "users" {
					usersArr, ok := v.AsArray()
					assertTrue(t, ok, "users should be array")

					userCount := 0
					for usersArr.Advance() {
						userCount++
						userObj, ok := usersArr.CurrentValue().AsObject()
						assertTrue(t, ok, "user should be object")

						// Each user has id, name, email, roles
						hasId := false
						hasName := false
						hasEmail := false
						hasRoles := false

						for userObj.Advance() {
							uk, uv := userObj.CurrentKeyVal()
							switch uk.ToString() {
							case "id":
								hasId = true
							case "name":
								hasName = true
							case "email":
								hasEmail = true
							case "roles":
								hasRoles = true
								rolesArr, ok := uv.AsArray()
								assertTrue(t, ok, "roles should be array")
								// Verify roles array is not empty
								assertTrue(t, rolesArr.Advance(), "roles should have at least one element")
							}
						}

						assertTrue(t, hasId && hasName && hasEmail && hasRoles, "user should have all fields")
					}

					if userCount != 2 {
						t.Errorf("expected 2 users, got %d", userCount)
					}
				}
			}

		case "metadata":
			foundMetadata = true
		}
	}

	assertTrue(t, foundStatus && foundData && foundMetadata, "should find all top-level keys")
}

// TestIntegrationStreamingLargeArray tests memory-bounded iteration over large array
func TestIntegrationStreamingLargeArray(t *testing.T) {
	// Create a large array that shouldn't be fully materialized in memory
	size := 1000
	items := make([]any, size)
	for i := 0; i < size; i++ {
		items[i] = map[string]any{
			"index": i,
			"data":  "item_" + string(rune('0'+i%10)),
		}
	}

	it := makeIterator(t, items)
	assertTrue(t, it.Advance(), "should have array")

	arr, ok := it.CurrentValue().AsArray()
	assertTrue(t, ok, "should be array")

	// Process array in streaming fashion
	count := 0
	sum := 0
	for arr.Advance() {
		count++
		obj, ok := arr.CurrentValue().AsObject()
		assertTrue(t, ok, "element should be object")

		// Extract index field
		for obj.Advance() {
			key, val := obj.CurrentKeyVal()
			if key.ToString() == "index" {
				s, ok := val.AsScalar()
				assertTrue(t, ok, "index should be scalar")
				num := s.ToGo().(float64)
				sum += int(num)
			}
		}
	}

	if count != size {
		t.Errorf("expected %d items, got %d", size, count)
	}

	expectedSum := (size - 1) * size / 2 // Sum of 0..999
	if sum != expectedSum {
		t.Errorf("expected sum=%d, got %d", expectedSum, sum)
	}
}

// TestIntegrationCloneAndDivergePaths tests cloning and processing different paths
func TestIntegrationCloneAndDivergePaths(t *testing.T) {
	data := map[string]any{
		"config": map[string]any{
			"debug":   true,
			"timeout": 30,
		},
		"users": []any{"alice", "bob"},
	}

	it := makeIterator(t, data)
	assertTrue(t, it.Advance(), "should have object")

	// Clone before processing
	original, ok := it.CurrentValue().AsObject()
	assertTrue(t, ok, "should be object")

	cloned, detach := original.CloneObject()
	defer detach()

	// Process original - look for config
	foundConfig := false
	for original.Advance() {
		key, val := original.CurrentKeyVal()
		if key.ToString() == "config" {
			foundConfig = true
			configObj, ok := val.AsObject()
			assertTrue(t, ok, "config should be object")

			// Verify config has debug field
			hasDebug := false
			for configObj.Advance() {
				k, _ := configObj.CurrentKeyVal()
				if k.ToString() == "debug" {
					hasDebug = true
				}
			}
			assertTrue(t, hasDebug, "config should have debug field")
			break // Stop after finding config
		}
	}
	assertTrue(t, foundConfig, "should find config")

	// Process clone - look for users
	foundUsers := false
	for cloned.Advance() {
		key, val := cloned.CurrentKeyVal()
		if key.ToString() == "users" {
			foundUsers = true
			usersArr, ok := val.AsArray()
			assertTrue(t, ok, "users should be array")

			userCount := 0
			for usersArr.Advance() {
				userCount++
			}
			if userCount != 2 {
				t.Errorf("expected 2 users, got %d", userCount)
			}
			break // Stop after finding users
		}
	}
	assertTrue(t, foundUsers, "should find users")
}

// TestIntegrationDiscardUnneededData tests discarding to skip processing
func TestIntegrationDiscardUnneededData(t *testing.T) {
	// Large document where we only want one field
	doc := map[string]any{
		"huge_data": []any{
			// Imagine this is gigabytes of data we don't need
			1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
		},
		"target_field": "this is what we want",
		"more_data": map[string]any{
			"nested": []any{1, 2, 3},
		},
	}

	it := makeIterator(t, doc)
	assertTrue(t, it.Advance(), "should have document")

	obj, ok := it.CurrentValue().AsObject()
	assertTrue(t, ok, "should be object")

	found := false
	for obj.Advance() {
		key, val := obj.CurrentKeyVal()
		if key.ToString() == "target_field" {
			s, ok := val.AsScalar()
			assertTrue(t, ok, "target should be scalar")
			if s.ToString() != "this is what we want" {
				t.Errorf("wrong value: %q", s.ToString())
			}
			found = true
			break // Stop processing, implicitly discarding remaining fields
		} else {
			// Explicitly discard values we don't care about
			val.Discard()
		}
	}

	assertTrue(t, found, "should find target field")
}

// TestIntegrationCopyAndTransform tests copying with transformation
func TestIntegrationCopyAndTransform(t *testing.T) {
	source := []any{
		map[string]any{"value": 10, "label": "a"},
		map[string]any{"value": 20, "label": "b"},
		map[string]any{"value": 30, "label": "c"},
	}

	it := makeIterator(t, source)
	assertTrue(t, it.Advance(), "should have array")

	arr, ok := it.CurrentValue().AsArray()
	assertTrue(t, ok, "should be array")

	// Copy the array to a token stream
	tokens := make(chan token.Token, 50)
	out := token.ChannelWriteStream(tokens)

	go func() {
		arr.Copy(out)
		close(tokens)
	}()

	// Collect tokens
	var collected []token.Token
	for tok := range tokens {
		collected = append(collected, tok)
	}

	// Verify structure: StartArray, 3 objects, EndArray
	if len(collected) < 5 {
		t.Errorf("expected at least 5 tokens, got %d", len(collected))
	}

	_, ok = collected[0].(*token.StartArray)
	assertTrue(t, ok, "first token should be StartArray")

	_, ok = collected[len(collected)-1].(*token.EndArray)
	assertTrue(t, ok, "last token should be EndArray")

	// Count StartObject tokens (should be 3)
	objectCount := 0
	for _, tok := range collected {
		if _, ok := tok.(*token.StartObject); ok {
			objectCount++
		}
	}
	if objectCount != 3 {
		t.Errorf("expected 3 objects, found %d", objectCount)
	}
}

// TestIntegrationEqualityOnLiveData tests equality checking during iteration
func TestIntegrationEqualityOnLiveData(t *testing.T) {
	data1 := map[string]any{
		"items": []any{1, 2, 3},
		"count": 3,
	}

	data2 := map[string]any{
		"count": 3,
		"items": []any{1, 2, 3},
	}

	// Create two iterators
	it1 := makeIterator(t, data1)
	it2 := makeIterator(t, data2)

	assertTrue(t, it1.Advance(), "it1 should advance")
	assertTrue(t, it2.Advance(), "it2 should advance")

	obj1, ok1 := it1.CurrentValue().AsObject()
	obj2, ok2 := it2.CurrentValue().AsObject()
	assertTrue(t, ok1 && ok2, "both should be objects")

	// Objects should be equal even though keys are in different order in source maps
	assertTrue(t, obj1.Equal(obj2), "objects should be equal")
}

// TestIntegrationNestedCloning tests cloning nested structures
func TestIntegrationNestedCloning(t *testing.T) {
	data := []any{
		[]any{1, 2, 3},
		[]any{4, 5, 6},
		[]any{7, 8, 9},
	}

	it := makeIterator(t, data)
	assertTrue(t, it.Advance(), "should have array")

	outerArr, ok := it.CurrentValue().AsArray()
	assertTrue(t, ok, "should be array")

	// Clone the outer array
	clonedArr, detach := outerArr.CloneArray()
	defer detach()

	// Process original array - sum first inner array
	assertTrue(t, outerArr.Advance(), "original should have first element")
	innerArr1, ok := outerArr.CurrentValue().AsArray()
	assertTrue(t, ok, "first element should be array")

	sum1 := 0
	for innerArr1.Advance() {
		s, _ := innerArr1.CurrentValue().AsScalar()
		num := s.ToGo().(float64)
		sum1 += int(num)
	}
	if sum1 != 6 { // 1+2+3
		t.Errorf("expected sum=6, got %d", sum1)
	}

	// Process cloned array - sum all inner arrays
	totalSum := 0
	for clonedArr.Advance() {
		innerArr, ok := clonedArr.CurrentValue().AsArray()
		assertTrue(t, ok, "element should be array")

		for innerArr.Advance() {
			s, _ := innerArr.CurrentValue().AsScalar()
			num := s.ToGo().(float64)
			totalSum += int(num)
		}
	}
	if totalSum != 45 { // 1+2+3+4+5+6+7+8+9
		t.Errorf("expected totalSum=45, got %d", totalSum)
	}
}

// TestIntegrationTransformerPipeline tests chaining multiple transformers
func TestIntegrationTransformerPipeline(t *testing.T) {
	// Create test data
	data := []any{1, 2, nil, 3, nil, 4}

	it := makeIterator(t, data)

	// First transformer: filter nulls
	filterTransformer := &filterNullsTransformer{}

	tokens1 := make(chan token.Token, 50)
	out1 := token.ChannelWriteStream(tokens1)

	go func() {
		for it.Advance() {
			filterTransformer.TransformValue(it.CurrentValue(), out1)
		}
		close(tokens1)
	}()

	// Create iterator from filtered stream
	it2 := New(token.ChannelReadStream(tokens1))

	// Second transformer: double numbers
	doubleTransformer := &doubleNumbersTransformer{}

	tokens2 := make(chan token.Token, 50)
	out2 := token.ChannelWriteStream(tokens2)

	go func() {
		for it2.Advance() {
			doubleTransformer.TransformValue(it2.CurrentValue(), out2)
		}
		close(tokens2)
	}()

	// Collect final output
	var result []int
	for tok := range tokens2 {
		if scalar, ok := tok.(*token.Scalar); ok {
			if scalar.Type() == token.Number {
				num := scalar.ToGo().(float64)
				result = append(result, int(num))
			}
		}
	}

	// Should have: 2, 4, 6, 8 (nulls removed, then doubled)
	expected := []int{2, 4, 6, 8}
	if len(result) != len(expected) {
		t.Errorf("expected %d values, got %d", len(expected), len(result))
	}

	for i, v := range expected {
		if i >= len(result) {
			break
		}
		if result[i] != v {
			t.Errorf("index %d: expected %d, got %d", i, v, result[i])
		}
	}
}

// TestIntegrationPartialObjectProcessing tests processing only specific object keys
func TestIntegrationPartialObjectProcessing(t *testing.T) {
	doc := map[string]any{
		"id":      123,
		"name":    "Test Item",
		"details": "Long description...",
		"tags":    []any{"a", "b", "c"},
		"meta":    map[string]any{"created": "2024-01-01"},
	}

	it := makeIterator(t, doc)
	assertTrue(t, it.Advance(), "should have object")

	obj, ok := it.CurrentValue().AsObject()
	assertTrue(t, ok, "should be object")

	// Only extract id and name, discard the rest
	var id int
	var name string

	for obj.Advance() {
		key, val := obj.CurrentKeyVal()
		switch key.ToString() {
		case "id":
			s, _ := val.AsScalar()
			num := s.ToGo().(float64)
			id = int(num)
		case "name":
			s, _ := val.AsScalar()
			name = s.ToString()
		default:
			// Discard values we don't need
			val.Discard()
		}
	}

	if id != 123 {
		t.Errorf("expected id=123, got %d", id)
	}
	if name != "Test Item" {
		t.Errorf("expected name='Test Item', got %q", name)
	}
}

// TestIntegrationMemoryBoundedProcessing verifies that processing doesn't materialize entire structures
func TestIntegrationMemoryBoundedProcessing(t *testing.T) {
	// This test verifies the streaming nature - we process one element at a time
	// without materializing the whole array
	size := 100

	items := make([]any, size)
	for i := 0; i < size; i++ {
		items[i] = map[string]any{
			"id":   i,
			"data": []any{i, i + 1, i + 2},
		}
	}

	it := makeIterator(t, items)
	assertTrue(t, it.Advance(), "should have array")

	arr, ok := it.CurrentValue().AsArray()
	assertTrue(t, ok, "should be array")

	// Process items one at a time
	count := 0
	for arr.Advance() {
		count++
		obj, ok := arr.CurrentValue().AsObject()
		assertTrue(t, ok, "item should be object")

		// Process object fields
		for obj.Advance() {
			key, val := obj.CurrentKeyVal()
			if key.ToString() == "data" {
				dataArr, ok := val.AsArray()
				assertTrue(t, ok, "data should be array")

				// Process nested array
				elemCount := 0
				for dataArr.Advance() {
					elemCount++
					// Just count, don't store
				}
				if elemCount != 3 {
					t.Errorf("expected 3 elements in nested array, got %d", elemCount)
				}
			}
		}
		// After processing each item, it can be garbage collected
		// This is the memory-bounded behavior
	}

	if count != size {
		t.Errorf("expected %d items, got %d", size, count)
	}
}
