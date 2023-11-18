package jsonstream

import (
	"fmt"

	"github.com/arnodel/jsonstream/iterator"
	"github.com/arnodel/jsonstream/token"
)

// A JSONEncoder can output a stream encoding a (stream of) JSON values
// using the given Printer instance for formatting.
type JSONEncoder struct {
	Printer
	*Colorizer
}

var _ token.StreamSink = &JSONEncoder{}

// Consume formats the JSON stream encoded in the given channel using the
// instance's Printer.  It assumes that the stream is well-formed, i.e.
// is a valid encoding for a stream of JSON values and may panic if that is
// not the case.
//
// And error can be returned if the Printer could not perform some writing
// operation.  A typical example is if it attempt to write to a closed pipe.
func (sw *JSONEncoder) Consume(stream <-chan token.Token) (err error) {
	defer CatchPrinterError(&err)
	iterator := iterator.New(token.ChannelReadStream(stream))
	for iterator.Advance() {
		sw.writeValue(iterator.CurrentValue())
		sw.Printer.Reset()
	}
	return nil
}

func (sw *JSONEncoder) writeValue(value iterator.Value) {
	switch v := value.(type) {
	case *iterator.Scalar:
		sw.Colorizer.PrintScalar(sw.Printer, v.Scalar())
	case *iterator.Object:
		sw.writeObject(v)
	case *iterator.Array:
		sw.writeArray(v)
	default:
		panic(fmt.Sprintf("invalid stream item: %#v", value))
	}
}

func (sw *JSONEncoder) writeObject(obj *iterator.Object) {
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
		sw.Colorizer.PrintScalar(sw.Printer, key)
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

func (sw *JSONEncoder) writeArray(arr *iterator.Array) {
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

var (
	elisionBytes           = []byte("...")
	openObjectBytes        = []byte("{")
	closeObjectBytes       = []byte("}")
	openArrayBytes         = []byte("[")
	closeArrayBytes        = []byte("]")
	itemSeparatorBytes     = []byte(",")
	keyValueSeparatorBytes = []byte(": ")
)
