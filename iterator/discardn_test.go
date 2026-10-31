package iterator

import (
	"testing"

	"github.com/arnodel/jsonstream/token"
)

// tokens for: [1, [2, 3], 4] 5
func discardTestStream() *Iterator {
	n := func(s string) *token.Scalar { return token.NewScalar(token.Number, []byte(s)) }
	return New(token.NewSliceReadStream([]token.Token{
		&token.StartArray{}, n("1"), &token.StartArray{}, n("2"), n("3"), &token.EndArray{}, n("4"), &token.EndArray{},
		n("5"),
	}))
}

func expectScalar(t *testing.T, iter *Iterator, want string) {
	t.Helper()
	if !iter.Advance() {
		t.Fatalf("expected a value %s, got end of stream", want)
	}
	s, ok := iter.CurrentValue().AsScalar()
	if !ok || string(s.Bytes) != want {
		t.Fatalf("expected scalar %s, got %v", want, iter.CurrentValue())
	}
}

func TestDiscardN_Partial(t *testing.T) {
	iter := discardTestStream()
	iter.Advance()
	arr := iter.CurrentValue()
	// Reads "1" and "[": not done.
	if n, done := DiscardN(arr, 2); n != 2 || done {
		t.Fatalf("DiscardN(2) = %d, %v; want 2, false", n, done)
	}
	// Reads 2, 3, ], 4, ]: done after 5 tokens even though 100 were allowed.
	if n, done := DiscardN(arr, 100); n != 5 || !done {
		t.Fatalf("DiscardN(100) = %d, %v; want 5, true", n, done)
	}
	if n, done := DiscardN(arr, 100); n != 0 || !done {
		t.Fatalf("DiscardN after done = %d, %v; want 0, true", n, done)
	}
	expectScalar(t, iter, "5")
}

func TestDiscardN_ResumedByAdvance(t *testing.T) {
	iter := discardTestStream()
	iter.Advance()
	arr, _ := iter.CurrentValue().AsArray()
	DiscardN(arr, 2)
	// A partially discarded collection is finished: Advance completes the
	// discard and reports the end.
	if arr.Advance() {
		t.Fatal("Advance after a partial discard should return false")
	}
	if !arr.Done() {
		t.Fatal("collection should be done after Advance")
	}
	expectScalar(t, iter, "5")
}

func TestDiscardN_ResumedByIterator(t *testing.T) {
	iter := discardTestStream()
	iter.Advance()
	DiscardN(iter.CurrentValue(), 3)
	// Iterator.Advance discards the rest of the current value first.
	expectScalar(t, iter, "5")
}

func TestDiscardN_PartiallyConsumedChild(t *testing.T) {
	iter := discardTestStream()
	iter.Advance()
	arr, _ := iter.CurrentValue().AsArray()
	arr.Advance() // 1
	arr.Advance() // [2, 3]
	child, _ := arr.CurrentValue().AsArray()
	child.Advance() // 2
	// The child is discarded first: "3" is read, child not done.
	if n, done := arr.DiscardN(1); n != 1 || done {
		t.Fatalf("DiscardN(1) = %d, %v; want 1, false", n, done)
	}
	// Then "]" finishes the child, and 4, ] finish the array.
	if n, done := arr.DiscardN(10); n != 3 || !done {
		t.Fatalf("DiscardN(10) = %d, %v; want 3, true", n, done)
	}
	expectScalar(t, iter, "5")
}

func TestDiscardN_Scalar(t *testing.T) {
	iter := discardTestStream()
	iter.Advance()
	iter.CurrentValue().Discard()
	expectScalar(t, iter, "5")
	if n, done := DiscardN(iter.CurrentValue(), 10); n != 0 || !done {
		t.Fatalf("DiscardN on scalar = %d, %v; want 0, true", n, done)
	}
}

func TestDiscardN_EndOfStream(t *testing.T) {
	// An unterminated array: the stream ends before the collection does.
	iter := New(token.NewSliceReadStream([]token.Token{&token.StartArray{}, token.NullScalar}))
	iter.Advance()
	if n, done := DiscardN(iter.CurrentValue(), 10); n != 1 || !done {
		t.Fatalf("DiscardN = %d, %v; want 1, true", n, done)
	}
}
