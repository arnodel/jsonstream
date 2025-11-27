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

type CursorPool struct {
	stream       ReadStream
	window       []Token
	windowPos    int
	catchupCount int
	cursors      []*Cursor

	cursorPoolDebugData
}

func NewCursorPool(stream ReadStream) *CursorPool {
	c, ok := stream.(*Cursor)
	if ok {
		return c.pool
	}
	return &CursorPool{stream: stream}
}

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

func (c *Cursor) Clone() *Cursor {
	if c.pool == nil {
		return c
	}
	return c.pool.CloneCursor(c)
}

func (c *Cursor) Detach() {
	if c.pool != nil {
		c.pool.DetachCursor(c)
	}
}

func CloneReadStream(stream ReadStream) (*Cursor, *Cursor) {
	c, ok := stream.(*Cursor)
	if !ok {
		c = NewCursorPool(stream).NewCursor()
	}
	return c, c.Clone()
}
