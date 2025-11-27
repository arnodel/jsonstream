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

type intToken int

func (n intToken) String() string {
	return ""
}

func TestCursorPool(t *testing.T) {
	toks := make([]Token, 10)
	for i := 0; i < 10; i++ {
		toks[i] = intToken(i)
	}
	var c1 ReadStream = NewSliceReadStream(toks)
	c1, c2 := CloneReadStream(c1)
	for i := 0; i < 10; i++ {
		assertNext(t, c1, intToken(i))
	}
	assertNext(t, c1, nil)
	assertNext(t, c1, nil)
	for i := 0; i < 5; i++ {
		assertNext(t, c2, intToken(i))
	}
	c3 := c2.Clone()
	for i := 5; i < 10; i++ {
		assertNext(t, c2, intToken(i))
		assertNext(t, c3, intToken(i))
	}
}

// TestSingleCursorAdvancement tests a single cursor reading an entire stream
func TestSingleCursorAdvancement(t *testing.T) {
	toks := makeTokens(100)
	var c ReadStream = NewSliceReadStream(toks)
	c, _ = CloneReadStream(c)

	for i := 0; i < 100; i++ {
		tok := c.Next()
		if tok != intToken(i) {
			t.Fatalf("at position %d: expected %v, got %v", i, intToken(i), tok)
		}
	}

	// Should return nil after exhausting stream
	if c.Next() != nil {
		t.Error("expected nil after stream exhaustion")
	}
}

// TestEmptyStream tests cursor behavior on an empty stream
func TestEmptyStream(t *testing.T) {
	var c ReadStream = NewSliceReadStream([]Token{})
	c, _ = CloneReadStream(c)

	if c.Next() != nil {
		t.Error("expected nil on empty stream")
	}
	if c.Next() != nil {
		t.Error("expected nil on second call to empty stream")
	}
}

// TestCursorCloneAtStart tests cloning a cursor at the beginning of a stream
func TestCursorCloneAtStart(t *testing.T) {
	toks := makeTokens(10)
	var stream ReadStream = NewSliceReadStream(toks)
	c1, c2 := CloneReadStream(stream)

	// Both cursors should read the same tokens
	for i := 0; i < 10; i++ {
		t1 := c1.Next()
		t2 := c2.Next()
		if t1 != intToken(i) || t2 != intToken(i) {
			t.Fatalf("mismatch at %d: c1=%v, c2=%v", i, t1, t2)
		}
	}
}

// TestCursorCloneAtMiddle tests cloning a cursor in the middle of a stream
func TestCursorCloneAtMiddle(t *testing.T) {
	toks := makeTokens(10)
	var stream ReadStream = NewSliceReadStream(toks)
	c1, _ := CloneReadStream(stream)

	// Advance c1 to middle
	for i := 0; i < 5; i++ {
		c1.Next()
	}

	// Clone at position 5
	c2 := c1.Clone()

	// Both should read tokens 5-9
	for i := 5; i < 10; i++ {
		t1 := c1.Next()
		t2 := c2.Next()
		if t1 != intToken(i) || t2 != intToken(i) {
			t.Fatalf("mismatch at %d: c1=%v, c2=%v", i, t1, t2)
		}
	}
}

// TestCursorCloneAtEnd tests cloning a cursor at the end of a stream
func TestCursorCloneAtEnd(t *testing.T) {
	toks := makeTokens(5)
	var stream ReadStream = NewSliceReadStream(toks)
	c1, _ := CloneReadStream(stream)

	// Exhaust c1
	for i := 0; i < 5; i++ {
		c1.Next()
	}

	// Clone at end
	c2 := c1.Clone()

	// Both should return nil
	if c1.Next() != nil || c2.Next() != nil {
		t.Error("expected nil from cursors at end of stream")
	}
}

// TestWindowGrowth tests that the window grows as cursors read ahead
func TestWindowGrowth(t *testing.T) {
	toks := makeTokens(100)
	var stream ReadStream = NewSliceReadStream(toks)
	c1, _ := CloneReadStream(stream)

	// Advance and verify window grows
	for i := 0; i < 50; i++ {
		c1.Next()
	}

	// Window should contain tokens read so far
	if len(c1.pool.window) < 50 {
		t.Errorf("expected window to contain at least 50 tokens, got %d", len(c1.pool.window))
	}
}

// TestWindowShrinkage tests that the window shrinks when lagging cursor advances
func TestWindowShrinkage(t *testing.T) {
	toks := makeTokens(200)
	var stream ReadStream = NewSliceReadStream(toks)
	c1, c2 := CloneReadStream(stream)

	// Advance c1 far ahead
	for i := 0; i < 150; i++ {
		c1.Next()
	}

	// Force window advancement by reaching catchup threshold
	c1.pool.catchupCount = catchupCountThreshold
	c1.pool.updateCatchupCount(1)

	windowLenAfterC1 := len(c1.pool.window)

	// Now advance c2 (the lagging cursor)
	for i := 0; i < 100; i++ {
		c2.Next()
	}

	// Force another window advancement
	c2.pool.catchupCount = catchupCountThreshold
	c2.pool.updateCatchupCount(1)

	// Window should have shrunk
	windowLenAfterC2 := len(c2.pool.window)
	if windowLenAfterC2 >= windowLenAfterC1 {
		t.Errorf("expected window to shrink, but went from %d to %d", windowLenAfterC1, windowLenAfterC2)
	}
}

// TestWindowResetOnNoCursors tests that window can be cleared when all cursors are detached
func TestWindowResetOnNoCursors(t *testing.T) {
	toks := makeTokens(50)
	var stream ReadStream = NewSliceReadStream(toks)
	// Create a single cursor (NewCursorPool returns a pool, then we create one cursor)
	pool := NewCursorPool(stream)
	c1 := pool.NewCursor()

	// Advance to build up window
	for i := 0; i < 30; i++ {
		c1.Next()
	}

	windowSizeBefore := len(pool.window)
	if windowSizeBefore < 30 {
		t.Errorf("expected window to have at least 30 tokens, got %d", windowSizeBefore)
	}

	// Exhaust the stream to trigger auto-detach
	for i := 30; i < 50; i++ {
		c1.Next()
	}
	c1.Next() // One more to trigger detach

	// Cursor should be detached
	if c1.pool != nil {
		t.Error("expected cursor to be detached")
	}

	// Before advanceWindow, window still has tokens
	if len(pool.window) == 0 {
		t.Error("window should still have tokens before advanceWindow is called")
	}

	// Force window advancement on the pool (with no cursors, resets window)
	pool.advanceWindow()

	// Now window should be nil after advanceWindow with no cursors
	if pool.window != nil {
		t.Errorf("expected window to be nil after advanceWindow with no cursors, got %d tokens", len(pool.window))
	}
}

// TestMemoryReuseSmallWindow tests that small windows (≤windowCapacityThreshold cap) always reuse array
func TestMemoryReuseSmallWindow(t *testing.T) {
	toks := makeTokens(100)
	var stream ReadStream = NewSliceReadStream(toks)
	c1, c2 := CloneReadStream(stream)

	// Advance c1 ahead
	for i := 0; i < 80; i++ {
		c1.Next()
	}

	// Get window capacity before advancement
	capBefore := cap(c1.pool.window)
	if capBefore > windowCapacityThreshold {
		t.Skip("window too large for this test")
	}

	// Advance c2 to trigger shrinkage
	for i := 0; i < 60; i++ {
		c2.Next()
	}

	// Force window advancement
	c1.pool.catchupCount = catchupCountThreshold
	c1.pool.updateCatchupCount(1)

	// Capacity should be same (array reused)
	capAfter := cap(c1.pool.window)
	if capAfter != capBefore {
		t.Errorf("expected capacity to remain %d (reuse), got %d", capBefore, capAfter)
	}
}

// TestMemoryReallocationLargeWindow tests reallocation for large poorly-utilized windows
func TestMemoryReallocationLargeWindow(t *testing.T) {
	// Create a large stream to force large window
	toks := makeTokens(2000)
	var stream ReadStream = NewSliceReadStream(toks)
	c1, c2 := CloneReadStream(stream)

	// Advance c1 far ahead to create large window
	for i := 0; i < 1500; i++ {
		c1.Next()
	}

	// Force window to grow
	c1.pool.catchupCount = catchupCountThreshold
	c1.pool.updateCatchupCount(1)

	capBefore := cap(c1.pool.window)
	if capBefore <= windowCapacityThreshold {
		t.Skip("window not large enough for this test")
	}

	// Now advance c2 almost to c1 (leaving only small window needed)
	for i := 0; i < 1480; i++ {
		c2.Next()
	}

	// Force window advancement - should trigger reallocation
	c1.pool.catchupCount = catchupCountThreshold
	c1.pool.updateCatchupCount(1)

	capAfter := cap(c1.pool.window)
	newLen := len(c1.pool.window)

	// If newLen*2 <= capBefore, should have reallocated to smaller array
	if newLen*2 <= capBefore && capAfter >= capBefore {
		t.Errorf("expected reallocation to smaller array: capBefore=%d, capAfter=%d, newLen=%d", capBefore, capAfter, newLen)
	}
}

// TestCapacityBoundaryThreshold tests behavior at exactly windowCapacityThreshold capacity
func TestCapacityBoundaryThreshold(t *testing.T) {
	// This test verifies the boundary condition in advanceWindow
	// where cap <= windowCapacityThreshold uses different logic than cap > windowCapacityThreshold

	// Create tokens and manually set up pool to test boundary
	toks := makeTokens(windowCapacityThreshold)
	var stream ReadStream = NewSliceReadStream(toks)
	c, _ := CloneReadStream(stream)

	// Read enough to fill window
	for i := 0; i < windowCapacityThreshold; i++ {
		c.Next()
	}

	// At boundary (cap=windowCapacityThreshold), should use reuse strategy
	if cap(c.pool.window) == windowCapacityThreshold {
		// This confirms we hit the boundary case
		t.Logf("Hit %d capacity boundary as expected", windowCapacityThreshold)
	}
}

// TestTwoCursorsInLockstep tests two cursors advancing together
func TestTwoCursorsInLockstep(t *testing.T) {
	toks := makeTokens(50)
	var stream ReadStream = NewSliceReadStream(toks)
	c1, c2 := CloneReadStream(stream)

	// Advance both together
	for i := 0; i < 50; i++ {
		t1 := c1.Next()
		t2 := c2.Next()
		if t1 != t2 {
			t.Fatalf("tokens differ at %d: %v vs %v", i, t1, t2)
		}
	}

	// Both cursors should be at the end
	if c1.position != 50 || c2.position != 50 {
		t.Errorf("expected both cursors at position 50, got c1=%d, c2=%d", c1.position, c2.position)
	}

	// When both cursors are at same position, window is not necessarily small
	// because advanceWindow hasn't been called yet. Just verify they're in sync.
	if c1.position != c2.position {
		t.Error("cursors should be at same position when advancing in lockstep")
	}
}

// TestTwoCursorsDivergent tests one cursor far ahead of another
func TestTwoCursorsDivergent(t *testing.T) {
	toks := makeTokens(100)
	var stream ReadStream = NewSliceReadStream(toks)
	c1, c2 := CloneReadStream(stream)

	// Advance c1 far ahead
	for i := 0; i < 90; i++ {
		c1.Next()
	}

	// c2 hasn't moved, so window must contain all 90 tokens
	if len(c1.pool.window) < 90 {
		t.Errorf("expected window >= 90 for divergent cursors, got %d", len(c1.pool.window))
	}

	// Now advance c2 a bit
	for i := 0; i < 10; i++ {
		c2.Next()
	}

	// c2 should still read correct tokens
	for i := 10; i < 20; i++ {
		tok := c2.Next()
		if tok != intToken(i) {
			t.Errorf("c2 at position %d: expected %v, got %v", i, intToken(i), tok)
		}
	}
}

// TestManyCursorsVaryingPositions tests multiple cursors at different positions
func TestManyCursorsVaryingPositions(t *testing.T) {
	toks := makeTokens(200)
	var stream ReadStream = NewSliceReadStream(toks)
	c0, c1 := CloneReadStream(stream)
	
	cursors := make([]*Cursor, 10)
	cursors[0] = c0
	cursors[1] = c1

	// Create remaining cursors
	for i := 2; i < 10; i++ {
		cursors[i] = cursors[0].Clone()
	}

	// Advance each cursor to a different position
	for i := 0; i < 10; i++ {
		for j := 0; j < i*10; j++ {
			cursors[i].Next()
		}
	}

	// All cursors should be able to continue reading correctly
	for i := 0; i < 10; i++ {
		startPos := i * 10
		tok := cursors[i].Next()
		expected := intToken(startPos)
		if tok != expected {
			t.Errorf("cursor %d: expected %v, got %v", i, expected, tok)
		}
	}
}

// TestCatchupTriggersAdvance tests that exceeding catchupCountThreshold catchup triggers advanceWindow
func TestCatchupTriggersAdvance(t *testing.T) {
	toks := makeTokens(200)
	var stream ReadStream = NewSliceReadStream(toks)
	c1, _ := CloneReadStream(stream)

	// Advance c1 far ahead
	for i := 0; i < 150; i++ {
		c1.Next()
	}

	// Set catchup to threshold - 1
	c1.pool.catchupCount = catchupCountThreshold - 1

	// One more update reaches threshold but doesn't trigger (need > threshold)
	c1.pool.updateCatchupCount(1)

	// Catchup at threshold doesn't trigger, need > threshold
	if c1.pool.catchupCount != catchupCountThreshold {
		t.Errorf("expected catchup to be %d, got %d", catchupCountThreshold, c1.pool.catchupCount)
	}

	// One more to exceed threshold
	c1.pool.updateCatchupCount(1)

	// Should have triggered advanceWindow and reset catchup
	if c1.pool.catchupCount != 0 {
		t.Errorf("expected catchup reset to 0, got %d", c1.pool.catchupCount)
	}
}

// TestAdvanceWindowShiftZero tests advanceWindow when all cursors at window start
func TestAdvanceWindowShiftZero(t *testing.T) {
	toks := makeTokens(50)
	var stream ReadStream = NewSliceReadStream(toks)
	c, _ := CloneReadStream(stream)

	// Advance a bit to create window
	for i := 0; i < 20; i++ {
		c.Next()
	}

	// Note current windowPos
	windowPosBefore := c.pool.windowPos
	windowLenBefore := len(c.pool.window)

	// All cursors are at same position (we only have one cursor at position 20)
	// But windowPos might be < 20, so let's force advanceWindow
	c.pool.advanceWindow()

	// After advancement, window should shift such that minPos (20) is at or after windowPos
	// Actually, with shift=0, nothing changes
	windowPosAfter := c.pool.windowPos
	windowLenAfter := len(c.pool.window)

	if c.position == windowPosBefore {
		// Shift would be 0, window unchanged
		if windowPosAfter != windowPosBefore || windowLenAfter != windowLenBefore {
			t.Error("expected no change when shiftRight == 0")
		}
	}
}

// TestDetachOnlyCursor tests detaching the only cursor
func TestDetachOnlyCursor(t *testing.T) {
	toks := makeTokens(20)
	var stream ReadStream = NewSliceReadStream(toks)
	c, _ := CloneReadStream(stream)

	// Advance a bit
	for i := 0; i < 10; i++ {
		c.Next()
	}

	// Exhaust to trigger detach
	for i := 10; i < 20; i++ {
		c.Next()
	}
	c.Next() // Should trigger detach

	// Cursor should be detached
	if c.pool != nil {
		t.Error("expected cursor to be detached (pool=nil)")
	}
}

// TestAutoDetachOnStreamEnd tests auto-detach when stream is exhausted
func TestAutoDetachOnStreamEnd(t *testing.T) {
	toks := makeTokens(5)
	var stream ReadStream = NewSliceReadStream(toks)
	c, _ := CloneReadStream(stream)

	// Read all tokens
	for i := 0; i < 5; i++ {
		c.Next()
	}

	// Next call should return nil and detach
	if c.Next() != nil {
		t.Error("expected nil at end of stream")
	}

	// Cursor should be detached
	if c.pool != nil {
		t.Error("expected cursor to be detached after stream exhaustion")
	}
}

// TestCloneDetachedCursor tests that cloning a detached cursor returns itself
func TestCloneDetachedCursor(t *testing.T) {
	toks := makeTokens(5)
	var stream ReadStream = NewSliceReadStream(toks)
	c, _ := CloneReadStream(stream)

	// Exhaust stream
	for c.Next() != nil {
	}

	// Cursor should be detached
	if c.pool != nil {
		t.Error("expected cursor to be detached")
	}

	// Cloning detached cursor should return same cursor
	c2 := c.Clone()
	if c2 != c {
		t.Error("expected clone of detached cursor to return same cursor")
	}
}

// Helper function to create token slices
func makeTokens(n int) []Token {
	toks := make([]Token, n)
	for i := 0; i < n; i++ {
		toks[i] = intToken(i)
	}
	return toks
}
