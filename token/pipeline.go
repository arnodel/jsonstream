package token

// A Stream carries the tokens flowing between the stages of a pipeline.  For
// efficiency the tokens travel in batches: every channel operation costs about
// as much as producing a token, so moving them one by one would spend most of
// the time in the scheduler.  Batches never hold back output: a stage flushes
// its partial batch whenever it is about to block waiting for input (see
// BatchReader and FlushingReader), so a value reaches the end of the pipeline
// as soon as its producer has nothing more to do.
//
// Use Reader to read the tokens of a Stream one by one.
type Stream <-chan []Token

// Reader returns a ReadStream reading the tokens of s one by one.
func (s Stream) Reader() *BatchReader {
	return NewBatchReader(s)
}

const (
	// batchSize is the number of tokens sent in one channel operation.
	batchSize = 256

	// streamChannelBuffer is the number of batches a Stream can hold before
	// its producer blocks.
	streamChannelBuffer = 4
)

// A Flusher is a WriteStream that may hold tokens back (such as a BatchWriter)
// and can be told to send them on.
type Flusher interface {
	Flush()
}

// FlushFunc returns a function flushing w if w is a Flusher, and nil
// otherwise.  Sources use it to flush their output before blocking on input.
func FlushFunc(w WriteStream) func() {
	if f, ok := w.(Flusher); ok {
		return f.Flush
	}
	return nil
}

// A StreamTransformer can transform a json stream into another.
// Use the TransformStream function to apply it.
//
// Transform reads tokens from in until Next returns nil (or earlier, if it has
// nothing more to produce) and writes its output to out.  It does not need to
// flush out.
type StreamTransformer interface {
	Transform(in ReadStream, out WriteStream)
}

// A StreamSource produces a stream of tokens, typically by decoding some
// input.  Use StartStream to run it.
//
// A source that may block waiting for input (e.g. reading from a pipe) should
// flush out first, if it is a Flusher, so that what it has produced so far is
// not held back; FlushingReader does this for sources reading an io.Reader.
// Produce does not need to flush out when it returns.
type StreamSource interface {
	Produce(out WriteStream) error
}

// A StreamSink consumes a stream of tokens, typically by encoding it to some
// output.  Use ConsumeStream to run it.
type StreamSink interface {
	Consume(in ReadStream) error
}

// TransformStream applies the transformer to the incoming json stream,
// returning a new json stream.  This is always fast because the
// transformer is computed in a goroutine.
//
// If the transformer returns before consuming all its input, the stream it
// returns is closed and the upstream producer is left blocked.
func TransformStream(in Stream, transformer StreamTransformer) Stream {
	out := make(chan []Token, streamChannelBuffer)
	go func() {
		defer close(out)
		w := NewBatchWriter(out, batchSize)
		defer w.Flush()
		r := NewBatchReader(in)
		// Send what we have before waiting for more input, so a slow input
		// does not hold back output that is ready.
		r.BeforeBlock = w.Flush
		transformer.Transform(r, w)
	}()
	return out
}

// StartStream uses the source to start producing items and returns a new json
// stream where these items are produced.  This is always fast because the
// source is computed in a goroutine.
//
// As a source can produce errors, a handleError function can be provided.
func StartStream(source StreamSource, handleError func(error)) Stream {
	out := make(chan []Token, streamChannelBuffer)
	go func() {
		defer close(out)
		w := NewBatchWriter(out, batchSize)
		defer w.Flush()
		err := source.Produce(w)
		if err != nil && handleError != nil {
			handleError(err)
		}
	}()
	return out
}

// ConsumeStream feeds the stream to the sink and returns its error.
func ConsumeStream(in Stream, sink StreamSink) error {
	return sink.Consume(NewBatchReader(in))
}
