package legacy

import (
	"github.com/arnodel/jsonstream/iterator"
	"github.com/arnodel/jsonstream/token"
)

// KeyExtractor is a Transformer that transforms an object into the value
// associated with a particular key.
//
// Deprecated: Use JSONPath queries instead (e.g., $.key).
// This transformer is kept for backward compatibility in the CLI.
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
// Deprecated: Use JSONPath descendant queries instead (e.g., $..key).
// This transformer is kept for backward compatibility in the CLI.
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
