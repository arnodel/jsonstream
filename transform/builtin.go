package transform

import (
	"log"

	"github.com/arnodel/jsonstream/iterator"
	"github.com/arnodel/jsonstream/token"
)

// MaxDepthFilter is a Transformer that truncates the stream to a given depth.
// Collections which are more deeply nested than MaxDepth are elided
// (their contents is replaced with "..." in the examples below).
//
// E.g.
//
//	[1, 2, {"x": [3, 4], "y": 2}]
//
// At MaxDepth=0:
//
//	[...]
//
// At MaxDepth=1
//
//	[1, 2, {...}]
//
// At MaxDepth=2
//
//	[1, 2, {"x": [...], "y": 2}]
type MaxDepthFilter struct {
	MaxDepth int
}

// Transform implements the MaxDepthFilter tansform.
func (f *MaxDepthFilter) Transform(in token.ReadStream, out token.WriteStream) {
	depth := 0
	defer func() {
		// Do not let the elision hide a truncated input.
		if depth != 0 {
			panic(token.ErrTruncated)
		}
	}()
	for item := in.Next(); item != nil; item = in.Next() {
		postIncr := 0
		switch item.(type) {
		case *token.StartArray, *token.StartObject:
			postIncr++
		case *token.EndArray, *token.EndObject:
			depth--
		}
		if depth <= f.MaxDepth {
			out.Put(item)
		}
		if depth == f.MaxDepth && postIncr > 0 {
			out.Put(&token.Elision{})
		}
		depth += postIncr
	}
}

// ExplodeArray is a transformer that turns an array into a stream of values.
// It copies other types unchanged.
//
//	E.g.
//	 [1, 2, 3]        -> 1 2 3
//	 {"x": 2, "y": 5} -> {"x": 2, "y": 5}
type ExplodeArray struct{}

// TransformValue implements the ExplodeArray transform
func (f ExplodeArray) TransformValue(value iterator.Value, out token.WriteStream) {
	switch v := value.(type) {
	case *iterator.Array:
		for v.Advance() {
			v.CurrentValue().Copy(out)
		}
	default:
		value.Copy(out)
	}
}

// JoinStream is the reverse of ExplodeArray.  It turns a stream of values
// into a JSON array
//
// E.g.
//
//	1 2 3          -> [1, 2, 3]
//	[1, 2, 3]      -> [[1, 2, 3]]
//	<empty stream> -> []
type JoinStream struct{}

// Transform implements the JoinStream transform
func (f JoinStream) Transform(in token.ReadStream, out token.WriteStream) {
	out.Put(&token.StartArray{})
	depth := 0
	for item := in.Next(); item != nil; item = in.Next() {
		switch item.(type) {
		case *token.StartArray, *token.StartObject:
			depth++
		case *token.EndArray, *token.EndObject:
			depth--
		}
		out.Put(item)
	}
	if depth != 0 {
		// Closing the array would make the cut-off value look complete.
		panic(token.ErrTruncated)
	}
	out.Put(&token.EndArray{})
}

// TraceStream logs all the stream items and doesn't send any items on.
// It's useful for debugging streams
type TraceStream struct{}

// Transform implements the TraceStream transform
func (t TraceStream) Transform(in token.ReadStream, out token.WriteStream) {
	for item := in.Next(); item != nil; item = in.Next() {
		log.Printf("%s", item)
	}
}
