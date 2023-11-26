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
	var p valueProcessor = valueToChannelAdapter{out: out}
	for i := len(r.segments) - 1; i >= 0; i-- {
		p = segmentProcessor{
			SegmentRunner: r.segments[i],
			next:          p,
		}
	}
	iterator := iterator.New(token.ChannelReadStream(in))
	for iterator.Advance() {
		p.ProcessValue(iterator.CurrentValue())
	}
}

type valueProcessor interface {
	ProcessValue(value iterator.Value)
}

type valueToChannelAdapter struct {
	out chan<- token.Token
}

func (a valueToChannelAdapter) ProcessValue(value iterator.Value) {
	value.Copy(a.out)
}

type segmentProcessor struct {
	SegmentRunner
	next valueProcessor
}

func (p segmentProcessor) ProcessValue(value iterator.Value) {
	p.TransformValue(value, p.next)
}
