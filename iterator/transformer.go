package iterator

import "github.com/arnodel/jsonstream/token"

// A ValueTransformer can transform a StreamedValue into a json stream.
// Use the AsStreamTransformer function to turn it into a
// StreamTransformer which can then be applied.
type ValueTransformer interface {
	TransformValue(iter Value, out chan<- token.Token)
}

// AsStreamTransformer turns a ValueTransformer into a StreamTransformer,
// so it can be applied to a json stream.
func AsStreamTransformer(transformer ValueTransformer) token.StreamTransformer {
	return &valueTransformerAdapter{valueTransformer: transformer}
}

type valueTransformerAdapter struct {
	valueTransformer ValueTransformer
}

func (f *valueTransformerAdapter) Transform(in <-chan token.Token, out chan<- token.Token) {
	iterator := New(token.ChannelReadStream(in))
	for iterator.Advance() {
		f.valueTransformer.TransformValue(iterator.CurrentValue(), out)
	}
}
