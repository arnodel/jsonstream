package iterator

import (
	"fmt"

	"github.com/arnodel/jsonstream/token"
)

type Iterator struct {
	stream       token.ReadStream
	currentValue Value
}

func New(stream token.ReadStream) *Iterator {
	return &Iterator{stream: stream}
}

func (i *Iterator) Advance() (ok bool) {
	if i.currentValue != nil {
		i.currentValue.Discard()
	}
	nextItem := i.stream.Next()
	if nextItem == nil {
		i.currentValue = nil
		return false
	}
	i.currentValue = nextStreamedValue(nextItem, i.stream)
	return true
}

func (i *Iterator) CurrentValue() Value {
	return i.currentValue
}

type Value interface {
	MakeRestartable() RestartFunc
	Discard()
	Copy(out chan<- token.Token)
}

type Scalar token.Scalar

var _ Value = &Scalar{}

func (s *Scalar) MakeRestartable() RestartFunc {
	return nil
}

func (s *Scalar) Discard() {}

func (s *Scalar) Copy(out chan<- token.Token) {
	out <- (*token.Scalar)(s)
}

func (s *Scalar) Scalar() *token.Scalar {
	return (*token.Scalar)(s)
}

type Collection interface {
	Value
	Advance() bool
	Done() bool
	Elided() bool
	CurrentValue() Value
}

type collectionBase struct {
	startItem token.Token
	stream    token.ReadStream

	started bool
	done    bool
	elided  bool

	currentValue Value
}

type RestartFunc func()

func (c *collectionBase) MakeRestartable() RestartFunc {
	if c.started {
		panic("cannot make started collection restartable")
	}
	restartableStream := token.NewRestartableReadStream(c.stream)
	c.stream = restartableStream
	return func() {
		restartableStream.Restart()
		c.done = false
		c.started = false
		c.currentValue = nil
	}
}

func (c *collectionBase) Discard() {
	if c.done {
		return
	}
	if c.started {
		c.currentValue.Discard()
	}
	c.done = true
	depth := 0
	for {
		item := c.stream.Next()
		if item == nil {
			return
		}
		switch item.(type) {
		case *token.StartArray, *token.StartObject:
			depth++
		case *token.EndArray, *token.EndObject:
			depth--
		}
		if depth < 0 {
			return
		}
	}
}

func (c *collectionBase) Copy(out chan<- token.Token) {
	if c.started {
		panic("cannot copy a started iterator")
	}
	out <- c.startItem
	c.done = true
	depth := 0
	for {
		item := c.stream.Next()
		if item == nil {
			return
		}
		switch item.(type) {
		case *token.StartArray, *token.StartObject:
			depth++
		case *token.EndArray, *token.EndObject:
			depth--
		}
		out <- item
		if depth < 0 {
			return
		}
	}
}

func (c *collectionBase) Elided() bool {
	return c.elided
}

func (c *collectionBase) CurrentValue() Value {
	if c.done {
		panic("iterator done")
	}
	return c.currentValue
}

type Object struct {
	collectionBase
	currentKey *token.Scalar
}

func (o *Object) CurrentKeyVal() (*token.Scalar, Value) {
	if o.done {
		panic("iterator done")
	}
	return o.currentKey, o.currentValue
}

func (o *Object) Advance() bool {
	if o.done {
		return false
	}
	if o.started {
		o.currentValue.Discard()
	}
	item := o.stream.Next()
	if item == nil {
		panic("stream ended inside object - expected key")
	}
	switch v := item.(type) {
	case *token.Scalar:
		o.started = true
		o.currentKey = v
		item := o.stream.Next()
		if item == nil {
			panic("stream ended inside obejct - expected value")
		}
		o.currentValue = nextStreamedValue(item, o.stream)
		return true
	case *token.EndObject:
		o.done = true
		return false
	case *token.Elision:
		o.elided = true
		// After this we expect o.done to be true
		return o.Advance()
	default:
		panic(fmt.Sprintf("invalid stream %#v, %#v", item, o.stream))
	}
}

type Array struct {
	collectionBase
}

func (a *Array) Advance() bool {
	if a.done {
		return false
	}
	if a.started {
		a.currentValue.Discard()
	}
	item := a.stream.Next()
	if item == nil {
		panic("stream ended inside array")
	}
	switch item.(type) {
	case *token.EndArray:
		a.done = true
		return false
	case *token.Elision:
		a.elided = true
		return a.Advance()
		// After this we expect a.done to be true
	default:
		a.started = true
		a.currentValue = nextStreamedValue(item, a.stream)
		return true
	}
}

func nextStreamedValue(firstItem token.Token, stream token.ReadStream) Value {
	switch v := firstItem.(type) {
	case *token.StartArray:
		return &Array{
			collectionBase: collectionBase{startItem: firstItem, stream: stream},
		}
	case *token.StartObject:
		return &Object{
			collectionBase: collectionBase{startItem: firstItem, stream: stream},
		}
	case *token.Scalar:
		return (*Scalar)(v)
	default:
		panic(fmt.Sprintf("invalid stream %#v", firstItem))
	}
}
