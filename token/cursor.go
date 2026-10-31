package token

import (
	"math"

	"github.com/arnodel/jsonstream/internal/debug"
)

const (
	// windowCapacityThreshold is the capacity threshold for window memory management.
	// Windows with capacity <= this threshold always reuse their underlying array
	// when shrinking. Windows with larger capacity may allocate a new smaller array
	// if utilization is poor (newLen*2 <= capacity).
	windowCapacityThreshold = 1024

	// catchupCountThreshold is the number of cursor advancements before triggering
	// advanceWindow to reclaim memory. This prevents calling advanceWindow too
	// frequently while ensuring timely garbage collection.
	catchupCountThreshold = 100
)

// A CursorPool lets several readers go over the same token stream, each at
// its own pace, while keeping in memory only the part of the stream that one
// of them may still need.  It is what lets a JSONPath query look at a value
// before deciding what to do with it ($[?@.price > 100] reads the item to
// evaluate the condition, then copies the whole item), reorder results
// ($[1, 0] emits the second item before the first), or compare two values key
// by key, all without buffering the whole document.
//
// # The model
//
// The pool pulls tokens from its underlying stream on demand into a window,
// a slice of consecutive tokens starting at stream position windowPos.
// A Cursor is just a position in the stream.  Advancing a cursor returns the
// token at its position, reading it from the window if it is already there
// and pulling it from the stream (and appending it to the window) if it is
// not; so the window always ends at the position of the cursor furthest
// ahead.  Tokens before the position of the cursor furthest behind can never
// be read again, and the pool discards them (see advanceWindow), so the
// window spans exactly the stretch between the slowest and the fastest
// cursor.  Memory use is bounded by the span under examination, not by the
// size of the stream.
//
// Cloning a cursor is a copy of a position, so it is cheap and does not copy
// any tokens.  Clones of cursors from one pool share that pool: NewCursorPool
// on a Cursor returns the cursor's own pool rather than wrapping it in a new
// one, so nested clones all draw on the same window.
//
// # Rules for users
//
//   - A cursor that is no longer needed must be detached (Cursor.Detach),
//     otherwise its position pins the window and the pool retains every token
//     from there on, for as long as the stream lasts.  This is the one way to
//     leak memory with this design; the detach functions returned by
//     iterator.Value.Clone exist to make it hard to forget.  Use
//     ObservePools and LiveCursors to check for leaks in tests.
//   - A cursor that reaches the end of the stream detaches itself.
//   - A cursor captures a stream position, not the state of whatever is
//     reading it, which is why an iterator.Value can only be cloned before it
//     has been started.
//   - Tokens are never modified after they enter the window; cursors share
//     them.
//   - Pools and cursors are not safe for concurrent use.
//
// # Invariants
//
//   - window[i] is the token at stream position windowPos + i.
//   - Every attached cursor has windowPos <= position <= windowPos + len(window).
//   - The window is only shortened from the front, and only up to the lowest
//     cursor position (advanceWindow panics otherwise).
//
// # Cost
//
// Advancing a cursor through the window is a bounds check and a slice index.
// Every catchupCountThreshold advances the pool scans its cursors (there are
// rarely more than a handful) to find the lowest position and discards the
// tokens before it, reusing the window's array when it is small or well used
// and allocating a smaller one otherwise so that a large array from an
// earlier wide span can be collected.
//
// The design is a sliding window pinned by outstanding readers, the same idea
// as mark/release token buffers in parser generators or itertools.tee in
// Python, with reclamation driven by the cursor positions rather than by
// explicit releases.
type CursorPool struct {
	stream       ReadStream
	window       []Token
	windowPos    int
	catchupCount int
	cursors      []*Cursor

	cursorPoolDebugData
}

// NewCursorPool returns a pool reading from stream.  If stream is itself a
// Cursor, its pool is returned, so that cursors derived from one another all
// share a window.
func NewCursorPool(stream ReadStream) *CursorPool {
	c, ok := stream.(*Cursor)
	if ok {
		return c.pool
	}
	p := &CursorPool{stream: stream}
	if poolObserver != nil {
		poolObserver(p)
	}
	return p
}

// LiveCursors returns the number of cursors attached to the pool.  Each one
// pins the window at its position, so a pool whose readers are all finished
// should report 0 (a cursor that reaches the end of the stream detaches
// itself).
func (p *CursorPool) LiveCursors() int {
	return len(p.cursors)
}

// poolObserver, if set, is called with every pool created by NewCursorPool.
var poolObserver func(*CursorPool)

// ObservePools arranges for observer to be called with every pool created by
// NewCursorPool from now on, and returns a function restoring the previous
// observer.  It is meant for tests: collect the pools, run a pipeline to
// completion, then check that LiveCursors is 0 on each, which is what proves
// that no cursor was left behind to pin the window.  Not safe for concurrent
// use with pipelines that create pools.
func ObservePools(observer func(*CursorPool)) (restore func()) {
	previous := poolObserver
	poolObserver = observer
	return func() { poolObserver = previous }
}

// NewCursorFromData returns a cursor over tokens already in memory.  Its pool
// has no stream to pull from and keeps the data for as long as the cursor is
// attached; it is not reported to ObservePools since there is nothing to
// reclaim.
func NewCursorFromData(data []Token) *Cursor {
	// A pool with just the data and a cursor pointing at the start.
	pool := &CursorPool{
		stream: NewSliceReadStream(nil),
		window: data,
	}
	cursor := &Cursor{pool: pool}
	pool.cursors = append(pool.cursors, cursor)
	return cursor
}

// advanceWindow shrinks the token window by discarding tokens that no cursor needs anymore.
//
// The CursorPool maintains a sliding window of tokens over the stream to enable multiple
// independent cursors to read the same stream efficiently. This method is called periodically
// (via updateCatchupCount after catchupCountThreshold cursor advances) to free memory by removing
// tokens that all cursors have already consumed.
//
// Algorithm:
//  1. Find the minimum cursor position (the leftmost/earliest cursor in the stream)
//  2. Calculate shiftRight = minPos - windowPos (number of tokens to discard from window start)
//  3. Update windowPos to reflect the new window start position
//  4. Either reuse the existing array (if small or well-utilized) or allocate a new smaller array
//
// Memory Optimization Strategy:
// The method uses different strategies based on window size and utilization:
//   - For small windows (cap ≤ windowCapacityThreshold): Always reuse the existing array for efficiency
//   - For large windows with good utilization (newLen*2 > cap): Reuse existing array
//   - For large windows with poor utilization (newLen*2 ≤ cap): Allocate new smaller array
//     to allow the garbage collector to reclaim the old large array
//
// Invariants Maintained:
//   - All cursor positions must be ≥ windowPos (enforced with panic if violated)
//   - window[i] corresponds to stream position windowPos + i
//   - Tokens are never modified after being added to the window
//   - The window contains all tokens from minCursorPos onwards
func (p *CursorPool) advanceWindow() {
	p.checkWindowSize()
	minPos := math.MaxInt
	for _, c := range p.cursors {
		if c.position < minPos {
			minPos = c.position
		}
	}
	if minPos == math.MaxInt {
		// There are no valid cursors - it is safe to reset the window
		p.windowPos += len(p.window)
		p.window = nil
		return
	}
	shiftRight := minPos - p.windowPos
	if shiftRight < 0 {
		panic("logic error")
	}
	if shiftRight == 0 {
		return
	}
	newLen := len(p.window) - shiftRight

	p.windowPos += shiftRight
	// If the reduced window is big enough, we reuse the same underlying array
	// for the new window slice, otherwise we make a new slice so the current
	// big slice can be GCed.
	if cap(p.window) <= windowCapacityThreshold || newLen*2 > cap(p.window) {
		copy(p.window, p.window[shiftRight:])
		p.window = p.window[:newLen]
	} else {
		debug.Printf("reducing window capacity %d to %d", cap(p.window), newLen)
		newWindow := make([]Token, newLen)
		copy(newWindow, p.window[shiftRight:])
		p.window = newWindow
	}
}

// We want this inlined
func (p *CursorPool) updateCatchupCount(n int) {
	p.catchupCount += n
	if p.catchupCount > catchupCountThreshold {
		p.catchupCount = 0
		p.advanceWindow()
	}
}

func (p *CursorPool) NewCursor() *Cursor {
	c := &Cursor{
		pool:     p,
		position: p.windowPos + len(p.window),
	}
	p.cursors = append(p.cursors, c)
	return c
}

func (p *CursorPool) CloneCursor(c *Cursor) *Cursor {
	if c == nil {
		return nil
	}
	clone := *c
	p.cursors = append(p.cursors, &clone)
	return &clone
}

func (p *CursorPool) DetachCursor(c *Cursor) {
	for i, c1 := range p.cursors {
		if c1 == c {
			p.updateCatchupCount(c.position - p.windowPos)
			newLen := len(p.cursors) - 1
			copy(p.cursors[i:], p.cursors[i+1:])
			p.cursors[newLen] = nil
			p.cursors = p.cursors[:newLen]
		}
	}
	c.pool = nil
}

func (p *CursorPool) AdvanceCursor(c *Cursor) Token {
	// TODO: optimize for when there is 1 cursor and empty window
	i := c.position - p.windowPos
	if i < len(p.window) {
		c.position++
		defer p.updateCatchupCount(1)
		return p.window[i]
	}
	if i > len(p.window) {
		panic("logic error")
	}
	tok := p.stream.Next()
	if tok != nil {
		c.position++
		p.window = append(p.window, tok)
	} else {
		p.DetachCursor(c)
	}
	return tok
}

// A Cursor is a position in the stream of a CursorPool, and a ReadStream: Next
// returns the token at the position and moves on.  See CursorPool for the
// model and the rules, in particular that a cursor must be detached when it is
// no longer needed.
type Cursor struct {
	pool     *CursorPool
	position int
}

var _ ReadStream = &Cursor{}

func (c *Cursor) Next() Token {
	if c.pool == nil {
		return nil
	}
	return c.pool.AdvanceCursor(c)
}

// Clone returns a new cursor at the same position, sharing the pool.  The
// clone must be detached when it is no longer needed.
func (c *Cursor) Clone() *Cursor {
	if c.pool == nil {
		return c
	}
	return c.pool.CloneCursor(c)
}

// Detach removes the cursor from its pool so that it no longer pins the
// window.  Next returns nil afterwards.  Detaching twice is harmless.
func (c *Cursor) Detach() {
	if c.pool != nil {
		c.pool.DetachCursor(c)
	}
}

// CloneReadStream returns two cursors at the current position of stream.  If
// stream is not already a Cursor it is wrapped in a new pool first; the first
// cursor returned then stands in for the original stream, the second is a
// clone to be detached when no longer needed.
func CloneReadStream(stream ReadStream) (*Cursor, *Cursor) {
	c, ok := stream.(*Cursor)
	if !ok {
		c = NewCursorPool(stream).NewCursor()
	}
	return c, c.Clone()
}
