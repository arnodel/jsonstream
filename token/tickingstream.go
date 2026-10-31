package token

import "time"

// TickingReadStream reads tokens from a channel.  While it is waiting for a
// token to arrive, it calls a callback at regular intervals.
//
// Its main purpose is to let a consumer flush buffered output when the input
// stalls (e.g. when following a log file), so that results that have already
// been computed reach the reader instead of sitting in a buffer until more
// input arrives.  The callback runs on the goroutine calling Next, so it can
// safely touch state owned by that goroutine.
type TickingReadStream struct {
	ch     <-chan Token
	ticker *time.Ticker
	onTick func()
}

var _ ReadStream = &TickingReadStream{}

// NewTickingReadStream returns a TickingReadStream reading from ch which calls
// onTick roughly every interval while waiting for a token.  Stop must be called
// when the stream is no longer needed.
func NewTickingReadStream(ch <-chan Token, interval time.Duration, onTick func()) *TickingReadStream {
	return &TickingReadStream{
		ch:     ch,
		ticker: time.NewTicker(interval),
		onTick: onTick,
	}
}

// Next returns the next token, or nil when the channel is closed.
func (r *TickingReadStream) Next() Token {
	for {
		select {
		case tok := <-r.ch:
			return tok
		case <-r.ticker.C:
			r.onTick()
		}
	}
}

// Stop releases the resources associated with the stream.  Next must not be
// called after Stop.
func (r *TickingReadStream) Stop() {
	r.ticker.Stop()
}
