package jsonstream

// A StreamTransformer can transform a json stream into another.
// Use the TransformStream function to apply it.
type StreamTransformer interface {
	Transform(in <-chan StreamItem, out chan<- StreamItem)
}

type StreamSource interface {
	Produce(chan<- StreamItem) error
}

type StreamSink interface {
	Consume(<-chan StreamItem) error
}

// A ValueTransformer can transform a StreamedValue into a json stream.
// Use the AsStreamTransformer function to turn it into a
// StreamTransformer which can then be applied.
type ValueTransformer interface {
	TransformValue(iter StreamedValue, out chan<- StreamItem)
}

// TransformStream applies the transformer to the incoming json stream,
// returning a new json stream.  This is always fast because the
// transformer is computed in a goroutine.
func TransformStream(in <-chan StreamItem, transformer StreamTransformer) <-chan StreamItem {
	out := make(chan StreamItem)
	go func() {
		defer close(out)
		transformer.Transform(in, out)
	}()
	return out
}

// StartStream uses the source to start producing items and returns a new json
// stream where these items are produced.  This is always fast because the
// source is computed in a goroutine.
//
// As a source can produce errors, a handleError function can be provided.
func StartStream(source StreamSource, handleError func(error)) <-chan StreamItem {
	out := make(chan StreamItem)
	go func() {
		defer close(out)
		err := source.Produce(out)
		if err != nil && handleError != nil {
			handleError(err)
		}
	}()
	return out
}

func ConsumeStream(in <-chan StreamItem, sink StreamSink) error {
	return sink.Consume(in)
}

// AsStreamTransformer turns a ValueTransformer into a StreamTransformer,
// so it can be applied to a json stream.
func AsStreamTransformer(transformer ValueTransformer) StreamTransformer {
	return &valueTransformerAdapter{valueTransformer: transformer}
}

type valueTransformerAdapter struct {
	valueTransformer ValueTransformer
}

func (f *valueTransformerAdapter) Transform(in <-chan StreamItem, out chan<- StreamItem) {
	iterator := NewStreamIterator(in)
	for iterator.Advance() {
		f.valueTransformer.TransformValue(iterator.CurrentValue(), out)
	}
}
