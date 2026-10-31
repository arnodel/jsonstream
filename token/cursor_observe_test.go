package token

import "testing"

func TestObservePoolsAndLiveCursors(t *testing.T) {
	var pools []*CursorPool
	restore := ObservePools(func(p *CursorPool) { pools = append(pools, p) })
	defer restore()

	toks := []Token{&StartArray{}, Int64Scalar(1), Int64Scalar(2), &EndArray{}}
	root, clone := CloneReadStream(NewSliceReadStream(toks))
	if len(pools) != 1 {
		t.Fatalf("expected 1 pool observed, got %d", len(pools))
	}
	p := pools[0]
	if p.LiveCursors() != 2 {
		t.Fatalf("expected 2 live cursors, got %d", p.LiveCursors())
	}
	// Cloning a cursor does not create a pool.
	clone2 := clone.Clone()
	if len(pools) != 1 || p.LiveCursors() != 3 {
		t.Fatalf("after cloning a cursor: pools=%d live=%d, want 1 and 3", len(pools), p.LiveCursors())
	}
	clone2.Detach()
	clone2.Detach() // harmless
	clone.Detach()
	if p.LiveCursors() != 1 {
		t.Fatalf("expected 1 live cursor after detaching clones, got %d", p.LiveCursors())
	}
	// Reading to the end detaches the root cursor by itself.
	for root.Next() != nil {
	}
	if p.LiveCursors() != 0 {
		t.Fatalf("expected 0 live cursors at end of stream, got %d", p.LiveCursors())
	}
	// A forgotten clone pins the window: this is the leak the observer is for.
	root2, leaked := CloneReadStream(NewSliceReadStream(toks))
	for root2.Next() != nil {
	}
	if pools[1].LiveCursors() != 1 {
		t.Fatalf("expected the undetached clone to be reported, got %d", pools[1].LiveCursors())
	}
	leaked.Detach()

	restore()
	CloneReadStream(NewSliceReadStream(toks))
	if len(pools) != 2 {
		t.Fatal("observer still called after restore")
	}
}
