package jsonstream

import (
	"fmt"
	"strconv"

	"github.com/arnodel/jsonstream/internal/format"
	"github.com/arnodel/jsonstream/iterator"
	"github.com/arnodel/jsonstream/token"
)

// A JPVEncoder can output a stream encoding a JSON value using the given
// Printer instance for formatting.  It prints the JSON value in the JPV format
// (see [JPVDecoder] for details of the format).
type JPVEncoder struct {
	format.Printer
	*format.Colorizer

	AlwaysQuoteKeys bool

	path []*token.Scalar // keeps track of the current path
}

var _ token.StreamSink = &JPVEncoder{}

// Consume formats the JSON stream encoded in the given channel using the
// instance's Printer.  It assumes that the stream is well-formed, i.e. is a
// valid encoding for a stream of JSON values and may panic if that is not the
// case.
//
// And error can be returned if the Printer could not perform some writing
// operation.  A typical example is if it attempt to write to a closed pipe.
func (e *JPVEncoder) Consume(stream <-chan token.Token) (err error) {
	defer format.CatchPrinterError(&err)
	iterator := iterator.New(token.ChannelReadStream(stream))
	for iterator.Advance() {
		e.writeValue(iterator.CurrentValue())
		e.path = e.path[:0]
		e.Reset()
	}
	return nil
}

func (e *JPVEncoder) writeValue(value iterator.Value) {
	switch v := value.(type) {
	case *iterator.Scalar:
		e.writeScalar(v.Scalar())
	case *iterator.Object:
		e.writeObject(v)
	case *iterator.Array:
		e.writeArray(v)
	default:
		panic(fmt.Sprintf("invalid stream item: %#v", value))
	}
}

func (e *JPVEncoder) writeObject(obj *iterator.Object) {
	var count = 0
	for obj.Advance() {
		key, value := obj.CurrentKeyVal()
		e.pushKey(key)
		e.writeValue(value)
		e.popKey()
		count++
	}
	if obj.Elided() || count == 0 {
		e.writePath()
		if !obj.Elided() {
			e.PrintBytes(emptyObjectBytes)
		} else if count != 0 {
			e.PrintBytes(elisionBytes)
		} else {
			e.PrintBytes(elidedBobjectBytes)
		}
		e.NewLine()
	}
}

func (e *JPVEncoder) writeArray(arr *iterator.Array) {
	var index = 0
	for arr.Advance() {
		e.pushKey(token.NewKey(token.Number, []byte(strconv.Itoa(index))))
		value := arr.CurrentValue()
		e.writeValue(value)
		e.popKey()
		index++
	}
	if arr.Elided() || index == 0 {
		e.writePath()
		if !arr.Elided() {
			e.PrintBytes(emptyArrayBytes)
		} else if index != 0 {
			e.PrintBytes(elisionBytes)
		} else {
			e.PrintBytes(elidedArrayBytes)
		}
		e.NewLine()
	}
}

func (e *JPVEncoder) writeScalar(scalar *token.Scalar) {
	e.writePath()
	e.PrintScalar(e.Printer, scalar)
	e.NewLine()
}

func (e *JPVEncoder) writePath() {
	e.PrintBytes(pathRootBytes)
	for _, key := range e.path {
		if key.IsAlnum() && !e.AlwaysQuoteKeys {
			e.PrintBytes([]byte{'.'})
			e.PrintSuccintScalar(e.Printer, key)
		} else {
			e.PrintBytes([]byte{'['})
			e.PrintScalar(e.Printer, key)
			e.PrintBytes([]byte{']'})
		}
	}
	e.PrintBytes(pathValueSeparatorBytes)
}

func (e *JPVEncoder) pushKey(key *token.Scalar) {
	e.path = append(e.path, key)
}

func (e *JPVEncoder) popKey() {
	e.path = e.path[:len(e.path)-1]
}

var (
	pathValueSeparatorBytes = []byte(" = ")
	pathRootBytes           = []byte("$")

	emptyObjectBytes   = []byte("{}")
	elidedBobjectBytes = []byte("{...}")
	emptyArrayBytes    = []byte("[]")
	elidedArrayBytes   = []byte("[...]")
)
