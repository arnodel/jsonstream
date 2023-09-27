package jsonstream

import (
	"fmt"
	"strconv"
)

// A JPVEncoder can output a stream encoding a JSON value using the given
// Printer instance for formatting.  It prints the JSON value in the JPV format
// (see [JPVDecoder] for details of the format).
type JPVEncoder struct {
	Printer

	path [][]byte // keeps track of the current path
}

var _ StreamSink = &JPVEncoder{}

// Consume formats the JSON stream encoded in the given channel using the
// instance's Printer.  It assumes that the stream is well-formed, i.e. is a
// valid encoding for a stream of JSON values and may panic if that is not the
// case.
//
// And error can be returned if the Printer could not perform some writing
// operation.  A typical example is if it attempt to write to a closed pipe.
func (e *JPVEncoder) Consume(stream <-chan StreamItem) (err error) {
	defer CatchPrinterError(&err)
	iterator := NewStreamIterator(stream)
	for iterator.Advance() {
		e.writeValue(iterator.CurrentValue())
		e.path = e.path[:0]
		e.Reset()
	}
	return nil
}

func (e *JPVEncoder) writeValue(value StreamedValue) {
	switch v := value.(type) {
	case *StreamedScalar:
		e.writeScalar(v.Scalar())
	case *StreamedObject:
		e.writeObject(v)
	case *StreamedArray:
		e.writeArray(v)
	default:
		panic(fmt.Sprintf("invalid stream item: %#v", value))
	}
}

func (e *JPVEncoder) writeObject(obj *StreamedObject) {
	var count = 0
	for obj.Advance() {
		key, value := obj.CurrentKeyVal()
		e.pushKey(key.Bytes)
		e.writeValue(value)
		e.popKey()
		count++
	}
	if obj.Elided() {
		if count == 0 {
			e.writePathWithValue(elidedBobjectBytes)
		} else {
			e.writePathWithValue(elisionBytes)
		}
	} else if count == 0 {
		e.writePathWithValue(emptyObjectBytes)
	}
}

func (e *JPVEncoder) writeArray(arr *StreamedArray) {
	var index = 0
	for arr.Advance() {
		e.pushKey([]byte(strconv.Itoa(index)))
		value := arr.CurrentValue()
		e.writeValue(value)
		e.popKey()
		index++
	}
	if arr.Elided() {
		if index == 0 {
			e.writePathWithValue(elidedArrayBytes)
		} else {
			e.writePathWithValue(elisionBytes)
		}
	} else if index == 0 {
		e.writePathWithValue(emptyArrayBytes)
	}
}

func (e *JPVEncoder) writeScalar(scalar *Scalar) {
	e.writePathWithValue(scalar.Bytes)
}

func (e *JPVEncoder) writePathWithValue(value []byte) {
	e.PrintBytes(pathRootBytes)
	for _, key := range e.path {
		e.PrintBytes([]byte{'['})
		e.PrintBytes(key)
		e.PrintBytes([]byte{']'})
	}
	e.PrintBytes(pathValueSeparatorBytes)
	e.PrintBytes(value)
	e.NewLine()
}

func (e *JPVEncoder) pushKey(key []byte) {
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
