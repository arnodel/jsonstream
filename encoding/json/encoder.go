package json

import (
	"fmt"
	"io"

	"github.com/arnodel/jsonstream/format"
	"github.com/arnodel/jsonstream/iterator"
	"github.com/arnodel/jsonstream/token"
)

// An Encoder writes a stream of JSON values as JSON text, pretty-printed,
// through a format.Printer.  NewEncoder sets one up with the defaults used by
// pj; the fields can be adjusted before Consume is called.
//
// Arrays are laid out in one of three forms depending on their contents and
// CompactWidthLimit: compact, with every item on one line when the scalars
// fit; mixed, where runs of scalars are packed onto lines up to the width
// limit while nested arrays and objects get lines of their own; and
// expanded, one item per line.  For example, with a limit of 20:
//
//	[
//	  1, 2, 3,
//	  {"k": "v"},
//	  4, 5, 6,
//	  [7, 8],
//	  9
//	]
//
// Objects are kept on one line when they have few, short, scalar members
// (CompactObjectMaxItems and CompactWidthLimit), and expanded otherwise.
type Encoder struct {
	// Printer receives the output.  A *format.DefaultPrinter with IndentSize
	// -1 writes each value on a single line (JSON Lines).
	format.Printer

	// Colorizer, if not nil, wraps scalars and keys in ANSI colour codes.
	*format.Colorizer

	// CompactWidthLimit is the width, in characters, up to which runs of
	// scalar array items are packed onto one line.  0 puts every item on its
	// own line.
	CompactWidthLimit int

	// CompactObjectMaxItems is the number of members up to which an object
	// whose members are all scalars, and fit in CompactWidthLimit, is kept
	// on one line.  0 expands every object.
	CompactObjectMaxItems int
}

var _ token.StreamSink = &Encoder{}

// NewEncoder returns an Encoder writing pretty-printed JSON to w: two spaces
// of indentation, with small arrays and objects kept on one line.  For JSON
// Lines output, set Printer to a format.DefaultPrinter with IndentSize -1.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{
		Printer:               &format.DefaultPrinter{Writer: w, IndentSize: 2},
		CompactWidthLimit:     60,
		CompactObjectMaxItems: 2,
	}
}

// Consume formats the JSON stream read from in using the
// instance's Printer.  It assumes that the stream is well-formed, i.e.
// is a valid encoding for a stream of JSON values and may panic if that is
// not the case.
//
// And error can be returned if the Printer could not perform some writing
// operation.  A typical example is if it attempt to write to a closed pipe.
func (sw *Encoder) Consume(in token.ReadStream) (err error) {
	defer format.CatchPrinterError(&err)
	iterator := iterator.New(in)
	for iterator.Advance() {
		sw.writeValue(iterator.CurrentValue())
		sw.Printer.Reset()
	}
	return nil
}

func (sw *Encoder) writeValue(value iterator.Value) {
	switch v := value.(type) {
	case *iterator.Scalar:
		sw.Colorizer.PrintScalar(sw.Printer, v.Scalar())
	case *iterator.Object:
		if sw.CompactObjectMaxItems > 0 {
			sw.writeObjectCompact(v)
		} else {
			sw.writeObject(v)
		}
	case *iterator.Array:
		if sw.CompactWidthLimit > 0 {
			sw.writeArrayCompact(v)
		} else {
			sw.writeArray(v)
		}
	default:
		panic(fmt.Sprintf("invalid stream item: %#v", value))
	}
}

func (sw *Encoder) writeObject(obj *iterator.Object) {
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

func (sw *Encoder) writeObjectCompact(obj *iterator.Object) {
	type keyValue struct {
		key   *token.Scalar
		value iterator.Value
	}
	pendingItems := make([]keyValue, 0, 10)
	totalWidth := -2
	compact := true
	for compact && obj.Advance() {
		key, value := obj.CurrentKeyVal()
		pendingItems = append(pendingItems, keyValue{key, value})
		scalar, isScalar := value.(*iterator.Scalar)
		if isScalar {
			totalWidth += len(key.Bytes) + len(scalar.Bytes) + 4 // 4 for ": " and ", "
		}
		compact = isScalar && len(pendingItems) <= sw.CompactObjectMaxItems && totalWidth <= sw.CompactWidthLimit
	}

	sw.PrintBytes(openObjectBytes)

	if compact {
		for i, item := range pendingItems {
			if i > 0 {
				sw.PrintBytes(compactItemSeparatorBytes)
			}
			sw.Colorizer.PrintScalar(sw.Printer, item.key)
			sw.PrintBytes(keyValueSeparatorBytes)
			sw.writeValue(item.value)
		}
		if obj.Elided() {
			sw.PrintBytes(elisionBytes)
		}
	} else {
		sw.Indent()
		for i, item := range pendingItems {
			if i > 0 {
				sw.PrintBytes(itemSeparatorBytes)
				sw.NewLine()
			}
			sw.Colorizer.PrintScalar(sw.Printer, item.key)
			sw.PrintBytes(keyValueSeparatorBytes)
			sw.writeValue(item.value)
		}
		for obj.Advance() {
			key, value := obj.CurrentKeyVal()

			// There was at least 1 pending item so we always want a separator
			sw.PrintBytes(itemSeparatorBytes)
			sw.NewLine()

			sw.Colorizer.PrintScalar(sw.Printer, key)
			sw.PrintBytes(keyValueSeparatorBytes)
			sw.writeValue(value)
		}
		if obj.Elided() {
			sw.NewLine()
			sw.PrintBytes(elisionBytes)
		}
		sw.Dedent()
	}

	sw.PrintBytes(closeObjectBytes)
}

func (sw *Encoder) writeArray(arr *iterator.Array) {
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

// Group together small scalar items up to a certain size
// E.g. with CompactSizeLimit = 20
//
//		  [1, 2, 3, 4]
//
//		  [
//		    "si", "par", "une",
//			"nuit", "d'hiver",
//			"un", "voyageur"
//		  ]
//
//	      [
//	         1, 2, 3, 4,
//	         [5, 6, 7],
//	         8, 9, 10, 11, 12
//	      ]
func (sw *Encoder) writeArrayCompact(arr *iterator.Array) {
	compactItems := make([]iterator.Value, 0, 10)
	totalWidth := -2
	sw.PrintBytes(openArrayBytes)
	firstItem := true
	for arr.Advance() {
		value := arr.CurrentValue()
		scalar, isScalar := value.(*iterator.Scalar)
		if isScalar {
			totalWidth += len(scalar.Bytes) + 2
		}
	AddValueToCompactItems:
		if isScalar && totalWidth <= sw.CompactWidthLimit {
			compactItems = append(compactItems, value)
			continue
		}
		if !firstItem {
			sw.PrintBytes(itemSeparatorBytes)
			sw.NewLine()
		} else {
			sw.Indent()
			firstItem = false
		}
		if len(compactItems) > 0 {
			for i, item := range compactItems {
				if i > 0 {
					sw.PrintBytes(compactItemSeparatorBytes)
				}
				sw.writeValue(item)
			}
			compactItems = compactItems[:0]

			if isScalar {
				totalWidth = len(scalar.Bytes)
				goto AddValueToCompactItems
			} else {
				totalWidth = -2
				sw.PrintBytes(itemSeparatorBytes)
				sw.NewLine()
			}
		}
		sw.writeValue(value)
	}
	if len(compactItems) > 0 {
		if !firstItem {
			sw.PrintBytes(itemSeparatorBytes)
			sw.NewLine()
		}
		for i, item := range compactItems {
			if i > 0 {
				sw.PrintBytes(compactItemSeparatorBytes)
			}
			sw.writeValue(item)
		}
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
	elisionBytes              = []byte("...")
	openObjectBytes           = []byte("{")
	closeObjectBytes          = []byte("}")
	openArrayBytes            = []byte("[")
	closeArrayBytes           = []byte("]")
	itemSeparatorBytes        = []byte(",")
	compactItemSeparatorBytes = []byte(", ")
	keyValueSeparatorBytes    = []byte(": ")
)
