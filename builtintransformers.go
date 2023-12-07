package jsonstream

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
func (f *MaxDepthFilter) Transform(in <-chan token.Token, out token.WriteStream) {
	depth := 0
	for item := range in {
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

// KeyExtractor is a Transformer that transforms an object into the value
// associated with a particular key.
//
// E.g. if the key is "name"
//
//	{"name": "Casimir", "color": "orange"} -> "Casimir"
//	{"id": 555, "status": "Done"}          -> <empty stream>
//	[1, 2, 3]                              -> <empty stream>
//	[{"name": "Kim"}, {"name": "Tim"}]     -> <empty stream>
type KeyExtractor struct {
	Key string
}

// TransformValue implements the KeyExtractor transform
func (f *KeyExtractor) TransformValue(value iterator.Value, out token.WriteStream) {
	if obj, ok := value.(*iterator.Object); ok {
		for obj.Advance() {
			key, val := obj.CurrentKeyVal()
			if key.EqualsString(f.Key) {
				val.Copy(out)
			}
		}
	}
}

// DeepKeyExtractor is a Transformer that finds all instances of a given key
// in the json stream and returns their associated values (in a stream)
//
// E.g. if the key is "id"
//
//	[{"id": 5, "parent": {"id": 2}}, {"id": 10}] -> 5 2 10
type DeepKeyExtractor struct {
	Key string
}

// TransformValue implements the DeepKeyExtractor transform
func (f *DeepKeyExtractor) TransformValue(value iterator.Value, out token.WriteStream) {
	switch v := value.(type) {
	case *iterator.Object:
		f.transformObject(v, out)
	case *iterator.Array:
		for v.Advance() {
			f.TransformValue(v.CurrentValue(), out)
		}
	}
}

func (f *DeepKeyExtractor) transformObject(obj *iterator.Object, out token.WriteStream) {
	for obj.Advance() {
		key, val := obj.CurrentKeyVal()
		if key.EqualsString(f.Key) {
			val.Copy(out)
		} else {
			f.TransformValue(val, out)
		}
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
func (f JoinStream) Transform(in <-chan token.Token, out token.WriteStream) {
	out.Put(&token.StartArray{})
	for item := range in {
		out.Put(item)
	}
	out.Put(&token.EndArray{})
}

// TraceStream logs all the stream items and doesn't send any items on.
// It's useful for debugging streams
type TraceStream struct{}

// Transform implements the TraceStream transform
func (t TraceStream) Transform(in <-chan token.Token, out token.WriteStream) {
	for item := range in {
		log.Printf("%s", item)
	}
}
