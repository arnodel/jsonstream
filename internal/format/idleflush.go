package format

import (
	"time"

	"github.com/arnodel/jsonstream/token"
)

// IdleFlushReadStream returns a token.ReadStream reading from ch which flushes
// flusher whenever no token has arrived for about interval.  This ensures that
// output already produced reaches its destination (e.g. a pipe to 'less') when
// the input stalls, without paying for a flush on every value when the input
// is flowing.
//
// The returned function must be called when the stream is no longer needed.
// If flusher is nil or interval is not positive, a plain channel stream is
// returned.
func IdleFlushReadStream(ch <-chan token.Token, flusher Flusher, interval time.Duration) (token.ReadStream, func()) {
	if flusher == nil || interval <= 0 {
		return token.ChannelReadStream(ch), func() {}
	}
	stream := token.NewTickingReadStream(ch, interval, func() {
		if err := flusher.Flush(); err != nil {
			panic(wrapError(err))
		}
	})
	return stream, stream.Stop
}
