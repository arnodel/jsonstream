package token

// A StreamTransformer can transform a json stream into another.
// Use the TransformStream function to apply it.
type StreamTransformer interface {
	Transform(in <-chan Token, out WriteStream)
}

type StreamSource interface {
	Produce(chan<- Token) error
}

type StreamSink interface {
	Consume(<-chan Token) error
}

// TransformStream applies the transformer to the incoming json stream,
// returning a new json stream.  This is always fast because the
// transformer is computed in a goroutine.
func TransformStream(in <-chan Token, transformer StreamTransformer) <-chan Token {
	out := make(chan Token)
	w := ChannelWriteStream(out)
	go func() {
		defer close(out)
		transformer.Transform(in, w)
	}()
	return out
}

// StartStream uses the source to start producing items and returns a new json
// stream where these items are produced.  This is always fast because the
// source is computed in a goroutine.
//
// As a source can produce errors, a handleError function can be provided.
func StartStream(source StreamSource, handleError func(error)) <-chan Token {
	out := make(chan Token)
	go func() {
		defer close(out)
		err := source.Produce(out)
		if err != nil && handleError != nil {
			handleError(err)
		}
	}()
	return out
}

func ConsumeStream(in <-chan Token, sink StreamSink) error {
	return sink.Consume(in)
}
