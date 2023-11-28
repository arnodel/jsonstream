package jsonpathtransformer

import (
	"github.com/arnodel/jsonstream/iterator"
	"github.com/arnodel/jsonstream/token"
)

//
// Query runner (still provisional)
//

type QueryRunner struct {
	isRootNodeQuery bool
	segments        []SegmentRunner
}

func (r QueryRunner) Transform(in <-chan token.Token, out chan<- token.Token) {
	p := r.getValueProcessor(valueToChannelAdapter{out: out})
	iterator := iterator.New(token.ChannelReadStream(in))
	for iterator.Advance() {
		p.ProcessValue(iterator.CurrentValue())
	}
}

func (r QueryRunner) EvaluateTruth(value iterator.Value) bool {
	var detach func()
	value, detach = value.Clone()
	if detach != nil {
		defer detach()
	}
	return !r.getValueProcessor(haltingProcessor{}).ProcessValue(value)
}

func (r QueryRunner) getValueProcessor(p valueProcessor) valueProcessor {
	for i := len(r.segments) - 1; i >= 0; i-- {
		p = segmentProcessor{
			SegmentRunner: r.segments[i],
			next:          p,
		}
	}
	return p
}

type valueProcessor interface {
	// ProcessValue processes the value and returns true if the caller should
	// continue, false if the caller can stop.
	ProcessValue(value iterator.Value) bool
}

type valueToChannelAdapter struct {
	out chan<- token.Token
}

func (a valueToChannelAdapter) ProcessValue(value iterator.Value) bool {
	value.Copy(a.out)
	return true
}

type segmentProcessor struct {
	SegmentRunner
	next valueProcessor
}

func (p segmentProcessor) ProcessValue(value iterator.Value) bool {
	return p.TransformValue(value, p.next)
}

type haltingProcessor struct{}

func (p haltingProcessor) ProcessValue(value iterator.Value) bool {
	return false
}
