package token

import (
	"testing"
)

func assertNext(t *testing.T, r ReadStream, expected Token) {
	next := r.Next()
	if next != expected {
		t.Fatalf("Expected %v, got %v", expected, next)
	}
}

func TestCursorPool(t *testing.T) {
	toks := make([]Token, 10)
	for i := 0; i < 10; i++ {
		toks[i] = i
	}
	var c1 ReadStream = NewSliceReadStream(toks)
	c1, c2 := CloneReadStream(c1)
	for i := 0; i < 10; i++ {
		assertNext(t, c1, i)
	}
	assertNext(t, c1, nil)
	assertNext(t, c1, nil)
	for i := 0; i < 5; i++ {
		assertNext(t, c2, i)
	}
	c3 := c2.Clone()
	for i := 5; i < 10; i++ {
		assertNext(t, c2, i)
		assertNext(t, c3, i)
	}
}
