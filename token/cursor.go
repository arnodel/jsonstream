package token

import (
	"math"

	"github.com/arnodel/jsonstream/internal/debug"
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
	if cap(p.window) <= 1024 || newLen*2 > cap(p.window) {
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
	if p.catchupCount > 100 {
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
