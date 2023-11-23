package jsonpathtransformer

import (
	"github.com/arnodel/jsonstream/iterator"
	"github.com/arnodel/jsonstream/token"
)

//
// Query runner (still provisional)
//

type QueryRunner interface {
	token.StreamTransformer
}

type RootNodeQueryRunner struct {
	segments []SegmentRunner
}

func (r RootNodeQueryRunner) Transform(in <-chan token.Token, out chan<- token.Token) {
	for _, segment := range r.segments {
		segmentTransformer := iterator.AsStreamTransformer(segment)
		in = token.TransformStream(in, segmentTransformer)
	}
	for token := range in {
		out <- token
	}
}
