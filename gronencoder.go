package jsonstream

import (
	"fmt"
	"strconv"
)

// A GRONEncoder can output a stream encoding a JSON value
// using the given Printer instance for formatting.  It prints the JSON
// value in the GRON format (see [GRONDecoder] for details of the format).
type GRONEncoder struct {
	Printer
	path [][]byte
}

var _ StreamSink = &GRONEncoder{}

// Consume formats the JSON stream encoded in the given channel using the
// instance's Printer.  It assumes that the stream is well-formed, i.e.
// is a valid encoding for a stream of JSON values and may panic if that is
// not the case.
//
// And error can be returned if the Printer could not perform some writing
// operation.  A typical example is if it attempt to write to a closed pipe.
func (sw *GRONEncoder) Consume(stream <-chan StreamItem) (err error) {
	defer CatchPrinterError(&err)
	iterator := NewStreamIterator(stream)
	for iterator.Advance() {
		sw.writeValue(iterator.CurrentValue())
		sw.path = sw.path[:0]
		sw.Reset()
	}
	return nil
}

func (sw *GRONEncoder) writeValue(value StreamedValue) {
	switch v := value.(type) {
	case *StreamedScalar:
		sw.writeScalar(v.Scalar())
	case *StreamedObject:
		sw.writeObject(v)
	case *StreamedArray:
		sw.writeArray(v)
	default:
		panic(fmt.Sprintf("invalid stream item: %#v", value))
	}
}

func (sw *GRONEncoder) writeObject(obj *StreamedObject) {
	for obj.Advance() {
		key, value := obj.CurrentKeyVal()
		sw.pushKey(key.Bytes)
		sw.writeValue(value)
		sw.popKey()
	}
	if obj.Elided() {
		sw.writePathWithValue(elisionBytes)
	}
}

func (sw *GRONEncoder) writeArray(arr *StreamedArray) {
	var index = 0
	for arr.Advance() {
		sw.pushKey([]byte(strconv.Itoa(index)))
		value := arr.CurrentValue()
		sw.writeValue(value)
		sw.popKey()
		index++
	}
	if arr.Elided() {
		sw.writePathWithValue(elisionBytes)
	}
}

func (sw GRONEncoder) writeScalar(scalar *Scalar) {
	sw.writePathWithValue(scalar.Bytes)
}

func (sw *GRONEncoder) writePathWithValue(value []byte) {
	sw.PrintBytes(pathRootBytes)
	for _, key := range sw.path {
		sw.PrintBytes([]byte{'['})
		sw.PrintBytes(key)
		sw.PrintBytes([]byte{']'})
	}
	sw.PrintBytes(pathValueSeparatorBytes)
	sw.PrintBytes(value)
	sw.NewLine()
}

func (sw *GRONEncoder) pushKey(key []byte) {
	sw.path = append(sw.path, key)
}

func (sw *GRONEncoder) popKey() {
	sw.path = sw.path[:len(sw.path)-1]
}

var (
	pathValueSeparatorBytes = []byte(" = ")
	pathRootBytes           = []byte(".")
)
