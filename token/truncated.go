package token

import "errors"

// ErrTruncated is the panic value (possibly wrapped) used by readers of a
// stream that ends in the middle of a value.  This happens when a source
// stops early because its input is invalid: it reports the error through
// StartStream's handleError and closes the stream, and every stage
// downstream then finds a value cut short.
//
// A stage that reaches the end of its input inside a value must panic with
// ErrTruncated rather than complete the value or carry on as if the input had
// ended normally: the iterator package does this, and so must transforms that
// copy tokens directly.  TransformStream recovers it and closes its own
// output, so the truncation travels down the pipeline without crashing
// anything; the final consumer can use CatchTruncated to turn it back into an
// error.
var ErrTruncated = errors.New("stream ended inside a value")

// CatchTruncated, when deferred, stops a panic caused by a truncated stream
// (see ErrTruncated) and stores its value in *err.  Any other panic carries
// on.
func CatchTruncated(err *error) {
	r := recover()
	if r == nil {
		return
	}
	if e, ok := r.(error); ok && errors.Is(e, ErrTruncated) {
		*err = e
		return
	}
	panic(r)
}
