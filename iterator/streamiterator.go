package iterator

import (
	"fmt"
	"math"
	"slices"

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

func (i *Iterator) Clone() (*Iterator, func()) {
	if i.currentValue != nil {
		panic("cannot clone started iterator")
	}
	clone := *i
	var cloneStream *token.Cursor
	i.stream, cloneStream = token.CloneReadStream(i.stream)
	clone.stream = cloneStream
	return &clone, cloneStream.Detach
}

type Value interface {
	Clone() (Value, func())
	Discard()
	Copy(token.WriteStream)
	AsScalar() (*token.Scalar, bool)
	AsArray() (*Array, bool)
	AsObject() (*Object, bool)
	Equal(Value) bool
}

type Scalar token.Scalar

var _ Value = &Scalar{}

func (s *Scalar) Clone() (Value, func()) {
	return s, nil
}

func (s *Scalar) Discard() {}

func (s *Scalar) Copy(w token.WriteStream) {
	w.Put((*token.Scalar)(s))
}

func (s *Scalar) Scalar() *token.Scalar {
	return (*token.Scalar)(s)
}

func (s *Scalar) AsScalar() (*token.Scalar, bool) {
	return s.Scalar(), true
}

func (s *Scalar) AsArray() (*Array, bool) {
	return nil, false
}

func (s *Scalar) AsObject() (*Object, bool) {
	return nil, false
}

func (s *Scalar) Equal(v Value) bool {
	vs, ok := v.(*Scalar)
	if !ok {
		return false
	}
	return s.Scalar().Equal(vs.Scalar())
}

type Collection interface {
	Value
	Advance() bool
	Done() bool
	Elided() bool
	CurrentValue() Value
	DiscardN(maxTokens int) (consumed int, done bool)
}

// DiscardN discards up to maxTokens tokens from the rest of v and reports how
// many tokens were read and whether the end of v was reached.  Scalars have
// nothing left to discard.  See Collection.DiscardN for details.
func DiscardN(v Value, maxTokens int) (consumed int, done bool) {
	if c, ok := v.(Collection); ok {
		return c.DiscardN(maxTokens)
	}
	return 0, true
}

type collectionBase struct {
	startItem token.Token
	stream    token.ReadStream

	started bool
	done    bool
	elided  bool

	currentValue Value

	// State of a discard in progress, see DiscardN.
	discarding      bool
	discardingChild Collection
	discardDepth    int

	// ownsStream is true when stream is a token.Cursor that this collection
	// created by cloning itself (see clone).  Such a cursor is detached when
	// the collection is finished, so that it does not stay attached to its
	// pool: a cursor only detaches itself when it reaches the end of the
	// stream, and the end of a value is usually not that.  Collections nested
	// in this one read from the same cursor but do not own it.
	ownsStream bool
}

// finish marks the end of the collection and releases its cursor if it owns
// one.
func (c *collectionBase) finish() {
	c.done = true
	if c.ownsStream {
		c.ownsStream = false
		if cursor, ok := c.stream.(*token.Cursor); ok {
			cursor.Detach()
		}
	}
}

func (c *collectionBase) clone() (collectionBase, func()) {
	if c.started {
		panic("cannot clone started collection")
	}
	clone := *c
	var cloneStream *token.Cursor
	_, wasCursor := c.stream.(*token.Cursor)
	c.stream, cloneStream = token.CloneReadStream(c.stream)
	// If the stream was not a cursor yet, CloneReadStream wrapped it in a
	// pool and the cursor now standing in for it is ours to release.
	c.ownsStream = c.ownsStream || !wasCursor
	clone.stream = cloneStream
	clone.ownsStream = false // the caller releases the clone with detach
	return clone, cloneStream.Detach
}

// Discard reads the rest of the collection from the stream so that the value
// following it can be read.
func (c *collectionBase) Discard() {
	c.DiscardN(math.MaxInt)
}

// DiscardN reads at most maxTokens tokens from the rest of the collection.  It
// returns the number of tokens read and whether the end of the collection was
// reached.
//
// Once DiscardN has been called the collection counts as finished: Advance
// discards whatever is left and returns false, and CurrentValue must not be
// called.  A partial discard can be resumed with further calls to DiscardN, or
// completed with Discard.
func (c *collectionBase) DiscardN(maxTokens int) (consumed int, done bool) {
	if c.done {
		return 0, true
	}
	if !c.discarding {
		c.discarding = true
		c.started = true // Copy must not be called from now on
		if child, ok := c.currentValue.(Collection); ok && !child.Done() {
			c.discardingChild = child
		}
	}
	if c.discardingChild != nil {
		n, childDone := c.discardingChild.DiscardN(maxTokens)
		consumed += n
		if !childDone {
			return consumed, false
		}
		c.discardingChild = nil
	}
	for consumed < maxTokens {
		item := c.stream.Next()
		if item == nil {
			c.finish()
			return consumed, true
		}
		consumed++
		switch item.(type) {
		case *token.StartArray, *token.StartObject:
			c.discardDepth++
		case *token.EndArray, *token.EndObject:
			c.discardDepth--
		}
		if c.discardDepth < 0 {
			c.finish()
			return consumed, true
		}
	}
	return consumed, false
}

func (c *collectionBase) Copy(w token.WriteStream) {
	if c.started {
		panic("cannot copy a started iterator")
	}
	w.Put(c.startItem)
	c.done = true
	depth := 0
	for {
		item := c.stream.Next()
		if item == nil {
			c.finish()
			panic(fmt.Errorf("%w: copying a collection", token.ErrTruncated))
		}
		switch item.(type) {
		case *token.StartArray, *token.StartObject:
			depth++
		case *token.EndArray, *token.EndObject:
			depth--
		}
		w.Put(item)
		if depth < 0 {
			c.finish()
			return
		}
	}
}

func (c *collectionBase) Elided() bool {
	return c.elided
}

// Done returns true once the end of the collection has been reached, either
// by advancing through all its items or by discarding or copying it.
func (c *collectionBase) Done() bool {
	return c.done
}

func (c *collectionBase) CurrentValue() Value {
	if c.done || c.discarding {
		panic("iterator done")
	}
	return c.currentValue
}

func (c *collectionBase) AsScalar() (*token.Scalar, bool) {
	return nil, false
}

func (c *collectionBase) AsArray() (*Array, bool) {
	return nil, false
}

func (c *collectionBase) AsObject() (*Object, bool) {
	return nil, false
}

var _ Collection = &Object{}
var _ Collection = &Array{}

type Object struct {
	collectionBase
	currentKey *token.Scalar
}

func (o *Object) CloneObject() (*Object, func()) {
	clone, detach := o.clone()
	return &Object{
		collectionBase: clone,
	}, detach
}

func (o *Object) Clone() (Value, func()) {
	return o.CloneObject()
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
	if o.discarding {
		o.Discard()
		return false
	}
	if o.started {
		o.currentValue.Discard()
	}
	item := o.stream.Next()
	if item == nil {
		panic(fmt.Errorf("%w: expected an object key", token.ErrTruncated))
	}
	switch v := item.(type) {
	case *token.Scalar:
		o.started = true
		o.currentKey = v
		item := o.stream.Next()
		if item == nil {
			panic(fmt.Errorf("%w: expected an object value", token.ErrTruncated))
		}
		o.currentValue = nextStreamedValue(item, o.stream)
		return true
	case *token.EndObject:
		o.finish()
		return false
	case *token.Elision:
		o.elided = true
		// After this we expect o.done to be true
		return o.Advance()
	default:
		panic(fmt.Sprintf("invalid stream %#v, %#v", item, o.stream))
	}
}

func (o *Object) AsObject() (*Object, bool) {
	return o, true
}

func (o *Object) Equal(v Value) bool {
	// Currently optimised for the case when the number of keys is small or the
	// keys are in a very similar order and the keys are unescaped because it
	// makes the implementation simple.  It's also probably good enough for many
	// cases, but can be very slow if both objects have many keys and they are
	// in very different orders.

	vo, ok := v.(*Object)
	if !ok {
		return false
	}

	type kvPair struct {
		key    *token.Scalar
		val    Value
		detach func()
	}

	var pending []kvPair // Stores key-values in right which haven't been matched yet

	defer func() {
		for _, p := range pending {
			if p.detach != nil {
				p.detach()
			}
		}
	}()

iterateLeft:
	for o.Advance() {
		key, val := o.CurrentKeyVal()
		for i, p := range pending {
			if !key.Equal(p.key) {
				continue
			}
			if !SafeValuesEqual(val, p.val) {
				return false
			}
			// We have matched the pending item with the current item from left.
			if p.detach != nil {
				p.detach()
			}
			pending = slices.Delete(pending, i, i+1)
			continue iterateLeft
		}
		// Not found in pending, so consume right until we find it
		for vo.Advance() {
			keyRight, valRight := vo.CurrentKeyVal()

			// If the key is not the one we want, store the key-value in pending
			// items
			if !key.Equal(keyRight) {
				valRightClone, detach := valRight.Clone()
				pending = append(pending, kvPair{keyRight, valRightClone, detach})
				continue
			}
			if !SafeValuesEqual(val, valRight) {
				return false
			}
			// We have matched!
			continue iterateLeft
		}
		// At this point, we have consumed the whole of right and not found a
		// matching key.
		return false
	}
	// The objects are equal if right has no more items
	return len(pending) == 0 && !vo.Advance()
}

type Array struct {
	collectionBase
}

func (a *Array) CloneArray() (*Array, func()) {
	clone, detach := a.clone()
	return &Array{collectionBase: clone}, detach
}

func (a *Array) Clone() (Value, func()) {
	return a.CloneArray()
}

func (a *Array) Advance() bool {
	if a.done {
		return false
	}
	if a.discarding {
		a.Discard()
		return false
	}
	if a.started {
		a.currentValue.Discard()
	}
	item := a.stream.Next()
	if item == nil {
		panic(fmt.Errorf("%w: expected an array item", token.ErrTruncated))
	}
	switch item.(type) {
	case *token.EndArray:
		a.finish()
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

func (a *Array) AsArray() (*Array, bool) {
	return a, true
}

func (a *Array) Equal(v Value) bool {
	va, ok := v.(*Array)
	if !ok {
		return false
	}
	for a.Advance() {
		if !va.Advance() {
			return false
		}
		if !a.CurrentValue().Equal(va.CurrentValue()) {
			return false
		}
	}
	// The arrays are equal if right has no more items.
	return !va.Advance()
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

// ValuesEqual returns true if v1 and v2 are equal.  It may advance v1 and / or v2.
func ValuesEqual(v1, v2 Value) bool {
	if v1 == nil || v2 == nil {
		return v1 == nil && v2 == nil
	}
	return v1.Equal(v2)
}

// SafeValuesEqual returns true if v1 and v2 are equal, without advancing either of
// them.
func SafeValuesEqual(v1, v2 Value) bool {
	if v1 == nil || v2 == nil {
		return v1 == nil && v2 == nil
	}
	// We could have a quick path for when left and right are scalars
	val1, detach1 := v1.Clone()
	val2, detach2 := v2.Clone()
	if detach1 != nil {
		defer detach1()
	}
	if detach2 != nil {
		defer detach2()
	}
	return val1.Equal(val2)
}
