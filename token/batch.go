package token

import (
	"io"
	"time"
)

// A BatchWriter is a WriteStream that collects tokens into batches and sends
// each batch on a channel when it is full or when Flush is called.
type BatchWriter struct {
	ch    chan<- []Token
	batch []Token
	size  int
}

var _ WriteStream = &BatchWriter{}
var _ Flusher = &BatchWriter{}

// NewBatchWriter returns a BatchWriter sending batches of up to size tokens
// on ch.
func NewBatchWriter(ch chan<- []Token, size int) *BatchWriter {
	return &BatchWriter{
		ch:    ch,
		batch: make([]Token, 0, size),
		size:  size,
	}
}

// Put adds a token to the current batch, sending the batch if it is full.
func (w *BatchWriter) Put(tok Token) {
	w.batch = append(w.batch, tok)
	if len(w.batch) == w.size {
		w.Flush()
	}
}

// Flush sends the current batch, if it is not empty.  The receiver owns the
// batch from then on, so a new one is started.
func (w *BatchWriter) Flush() {
	if len(w.batch) == 0 {
		return
	}
	w.ch <- w.batch
	w.batch = make([]Token, 0, w.size)
}

// A BatchReader is a ReadStream reading tokens one by one from a channel of
// batches, such as a Stream.
type BatchReader struct {
	ch     <-chan []Token
	batch  []Token
	pos    int
	closed bool

	// BeforeBlock, if not nil, is called when Next has to wait for the next
	// batch to arrive on the channel.  TransformStream uses it to flush the
	// output of the transformer before it blocks on its input.
	BeforeBlock func()

	// OnIdle, if not nil, is called when Next has been waiting for the next
	// batch for IdleTimeout.  It is called at most once per wait: its purpose
	// is to flush output buffered downstream (e.g. the bufio.Writer behind a
	// pipe) when the input stalls, and once that is done there is nothing
	// more to flush until input arrives.  It runs on the goroutine calling
	// Next.
	OnIdle      func()
	IdleTimeout time.Duration

	idleTimer *time.Timer
}

var _ ReadStream = &BatchReader{}

// NewBatchReader returns a BatchReader reading from ch.
func NewBatchReader(ch <-chan []Token) *BatchReader {
	return &BatchReader{ch: ch}
}

// Next returns the next token, or nil once the channel is closed and all its
// tokens have been returned.
func (r *BatchReader) Next() Token {
	if r.pos < len(r.batch) {
		tok := r.batch[r.pos]
		r.pos++
		return tok
	}
	if r.closed {
		return nil
	}
	var batch []Token
	var ok bool
	select {
	case batch, ok = <-r.ch:
	default:
		// Nothing ready yet: we are about to block.
		if r.BeforeBlock != nil {
			r.BeforeBlock()
		}
		batch, ok = r.waitForBatch()
	}
	if !ok {
		r.closed = true
		r.batch = nil
		r.pos = 0
		return nil
	}
	// Batches are never empty (see BatchWriter.Flush).
	r.batch = batch
	r.pos = 1
	return batch[0]
}

// waitForBatch blocks until a batch arrives (or the channel is closed), calling
// OnIdle if the wait exceeds IdleTimeout.
func (r *BatchReader) waitForBatch() ([]Token, bool) {
	if r.OnIdle == nil || r.IdleTimeout <= 0 {
		batch, ok := <-r.ch
		return batch, ok
	}
	if r.idleTimer == nil {
		r.idleTimer = time.NewTimer(r.IdleTimeout)
	} else {
		r.idleTimer.Reset(r.IdleTimeout)
	}
	select {
	case batch, ok := <-r.ch:
		r.idleTimer.Stop()
		return batch, ok
	case <-r.idleTimer.C:
		r.OnIdle()
		batch, ok := <-r.ch
		return batch, ok
	}
}

// A FlushingReader is an io.Reader that calls Flush, if set, before each Read
// of the underlying reader.  Decoders read their input through one so that
// the tokens they have produced so far are sent on before they block waiting
// for more input.
type FlushingReader struct {
	io.Reader
	Flush func()
}

// Read calls Flush, if set, then reads from the underlying reader.
func (r *FlushingReader) Read(p []byte) (int, error) {
	if r.Flush != nil {
		r.Flush()
	}
	return r.Reader.Read(p)
}
