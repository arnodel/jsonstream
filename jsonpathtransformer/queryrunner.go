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

func (r QueryRunner) Evaluate(value iterator.Value) bool {
	value = value.Clone()
	var p countingProcessor
	// TODO as soon as a value is received, stop processing.  This would require
	// something like ProcessValue returning a boolean to make the caller return.
	r.getValueProcessor(&p).ProcessValue(value)
	value.Discard()
	return p.count > 0
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

type countingProcessor struct {
	count int
}

func (p *countingProcessor) ProcessValue(value iterator.Value) {
	value.Discard()
	p.count++
}
