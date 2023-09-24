package jsonstream

import (
	"fmt"
)

type StreamIterator struct {
	stream       <-chan StreamItem
	currentValue StreamedValue
}

func NewStreamIterator(stream <-chan StreamItem) *StreamIterator {
	return &StreamIterator{stream: stream}
}

func (i *StreamIterator) Advance() (ok bool) {
	if i.currentValue != nil {
		i.currentValue.Discard()
	}
	nextItem, ok := <-i.stream
	if !ok {
		i.currentValue = nil
		return false
	}
	i.currentValue = nextStreamedValue(nextItem, i.stream)
	return true
}

func (i *StreamIterator) CurrentValue() StreamedValue {
	return i.currentValue
}

type StreamedValue interface {
	Discard()
	Copy(out chan<- StreamItem)
}

type StreamedScalar Scalar

var _ StreamedValue = &StreamedScalar{}

func (s *StreamedScalar) Discard() {}

func (s *StreamedScalar) Copy(out chan<- StreamItem) {
	out <- (*Scalar)(s)
}

func (s *StreamedScalar) Scalar() *Scalar {
	return (*Scalar)(s)
}

type StreamedCollection interface {
	StreamedValue
	Advance() bool
	Done() bool
	Elided() bool
	CurrentValue() StreamedValue
}

type streamedCollectionBase struct {
	startItem StreamItem
	stream    <-chan StreamItem

	started bool
	done    bool
	elided  bool

	currentValue StreamedValue
}

func (c *streamedCollectionBase) Discard() {
	if c.done {
		return
	}
	if c.started {
		c.currentValue.Discard()
	}
	c.done = true
	depth := 0
	for item := range c.stream {
		switch item.(type) {
		case *StartArray, *StartObject:
			depth++
		case *EndArray, *EndObject:
			depth--
		}
		if depth < 0 {
			return
		}
	}
}

func (c *streamedCollectionBase) Copy(out chan<- StreamItem) {
	if c.started {
		panic("cannot copy a started iterator")
	}
	out <- c.startItem
	c.done = true
	depth := 0
	for item := range c.stream {
		switch item.(type) {
		case *StartArray, *StartObject:
			depth++
		case *EndArray, *EndObject:
			depth--
		}
		out <- item
		if depth < 0 {
			return
		}
	}
}

func (c *streamedCollectionBase) Elided() bool {
	return c.elided
}

func (c *streamedCollectionBase) CurrentValue() StreamedValue {
	if c.done {
		panic("iterator done")
	}
	return c.currentValue
}

type StreamedObject struct {
	streamedCollectionBase
	currentKey *Scalar
}

func (o *StreamedObject) CurrentKeyVal() (*Scalar, StreamedValue) {
	if o.done {
		panic("iterator done")
	}
	return o.currentKey, o.currentValue
}

func (o *StreamedObject) Advance() bool {
	if o.done {
		return false
	}
	if o.started {
		o.currentValue.Discard()
	}
	item, ok := <-o.stream
	if !ok {
		panic("stream ended inside object - expected key")
	}
	switch v := item.(type) {
	case *Scalar:
		o.started = true
		o.currentKey = v
		item, ok := <-o.stream
		if !ok {
			panic("stream ended inside obejct - expected value")
		}
		o.currentValue = nextStreamedValue(item, o.stream)
		return true
	case *EndObject:
		o.done = true
		return false
	case *Elision:
		o.elided = true
		// After this we expect o.done to be true
		return o.Advance()
	default:
		panic(fmt.Sprintf("invalid stream %#v", item))
	}
}

type StreamedArray struct {
	streamedCollectionBase
}

func (a *StreamedArray) Advance() bool {
	if a.done {
		return false
	}
	if a.started {
		a.currentValue.Discard()
	}
	item, ok := <-a.stream
	if !ok {
		panic("stream ended inside array")
	}
	switch item.(type) {
	case *EndArray:
		a.done = true
		return false
	case *Elision:
		a.elided = true
		return a.Advance()
		// After this we expect a.done to be true
	default:
		a.started = true
		a.currentValue = nextStreamedValue(item, a.stream)
		return true
	}
}

func nextStreamedValue(firstItem StreamItem, stream <-chan StreamItem) StreamedValue {
	switch v := firstItem.(type) {
	case *StartArray:
		return &StreamedArray{
			streamedCollectionBase: streamedCollectionBase{startItem: firstItem, stream: stream}}
	case *StartObject:
		return &StreamedObject{
			streamedCollectionBase: streamedCollectionBase{startItem: firstItem, stream: stream}}
	case *Scalar:
		return (*StreamedScalar)(v)
	default:
		panic(fmt.Sprintf("invalid stream %#v", firstItem))
	}
}
