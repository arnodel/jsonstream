package jsonstream

import (
	"fmt"
)

// A JSONEncoder can output a stream encoding a (stream of) JSON values
// using the given Printer instance for formatting.
type JSONEncoder struct {
	Printer
}

var _ StreamSink = &JSONEncoder{}

// Consume formats the JSON stream encoded in the given channel using the
// instance's Printer.  It assumes that the stream is well-formed, i.e.
// is a valid encoding for a stream of JSON values and may panic if that is
// not the case.
//
// And error can be returned if the Printer could not perform some writing
// operation.  A typical example is if it attempt to write to a closed pipe.
func (sw *JSONEncoder) Consume(stream <-chan StreamItem) (err error) {
	defer CatchPrinterError(&err)
	iterator := NewStreamIterator(stream)
	for iterator.Advance() {
		sw.writeValue(iterator.CurrentValue())
		sw.Printer.NewLine()
	}
	return nil
}

func (sw *JSONEncoder) writeValue(value StreamedValue) {
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

func (sw *JSONEncoder) writeObject(obj *StreamedObject) {
	sw.PrintBytes(openObjectBytes)
	firstItem := true
	for obj.Advance() {
		key, value := obj.CurrentKeyVal()
		if !firstItem {
			sw.PrintBytes(itemSeparatorBytes)
			sw.NewLine()
		} else {
			sw.Indent()
			firstItem = false
		}
		sw.writeScalar(key)
		sw.PrintBytes(keyValueSeparatorBytes)
		sw.writeValue(value)
	}
	if obj.Elided() {
		if !firstItem {
			sw.NewLine()
		}
		sw.PrintBytes(elisionBytes)
	}
	if !firstItem {
		sw.Dedent()
	}
	sw.PrintBytes(closeObjectBytes)
}

func (sw *JSONEncoder) writeArray(arr *StreamedArray) {
	sw.PrintBytes(openArrayBytes)
	firstItem := true
	for arr.Advance() {
		value := arr.CurrentValue()
		if !firstItem {
			sw.PrintBytes(itemSeparatorBytes)
			sw.NewLine()
		} else {
			sw.Indent()
			firstItem = false
		}
		sw.writeValue(value)
	}
	if arr.Elided() {
		if !firstItem {
			sw.NewLine()
		}
		sw.PrintBytes(elisionBytes)
	}
	if !firstItem {
		sw.Dedent()
	}
	sw.PrintBytes(closeArrayBytes)
}

func (sw JSONEncoder) writeScalar(scalar *Scalar) {
	sw.PrintBytes(scalar.Bytes)
}

var (
	elisionBytes           = []byte("...")
	openObjectBytes        = []byte("{")
	closeObjectBytes       = []byte("}")
	openArrayBytes         = []byte("[")
	closeArrayBytes        = []byte("]")
	itemSeparatorBytes     = []byte(",")
	keyValueSeparatorBytes = []byte(": ")
)
