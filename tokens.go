package jsonstream

import (
	"bytes"
	"fmt"
)

// A Token is an item in a stream that encodes a JSON value
// For example, the JSON value
//
//	{"id": 123, "tags": ["important", "new"]}
//
// would be represented by the stream of Token (in pseudocode for
// clarity):
//
//	{            -> StartObject
//	"id":        -> Scalar("id", String)
//	123,         -> Scalar(123, Number)
//	"tags":      -> Scalar("tags", String)
//	[            -> StartArray
//	"important", -> Scalar("important", String)
//	"new"        -> Scalar("new", String)
//	]            -> EndArray
//	}            -> EndObject
//
// encoded of a JSON input into a stream of Token values, processing of
// this stream and outputting the outcome can be done concurrently using
// channels of Token values.
type Token interface{}

// StartObject represents the start of a JSON object (introduced by '{').
// Values of type *StartObject implement the StreamItem interface.
type StartObject struct{}

func (s *StartObject) String() string {
	return "StartObject"
}

var _ Token = &StartObject{}

// EndObject represents the end of a JSON object (introduced by '}')
// Values of type *EndObject implement the StreamItem interface.
type EndObject struct{}

func (e *EndObject) String() string {
	return "EndObject"
}

var _ Token = &EndObject{}

// StartArray represents the start of a JSON array (introduced by '[').
// Values of type *StartArray implement the StreamItem interface.
type StartArray struct{}

func (s *StartArray) String() string {
	return "StartArray"
}

var _ Token = &StartArray{}

// EndArray represents the end of a JSON array (introduced by '}')
// Values of type *EndArray implement the StreamItem interface.
type EndArray struct{}

func (e *EndArray) String() string {
	return "EndArray"
}

var _ Token = &EndArray{}

// Elision is not part of the JSON syntax but is used to remove contents
// from an array or an object but signal to the user that the content has
// been 'elided'.
type Elision struct{}

func (e *Elision) String() string {
	return "Elision"
}

var _ Token = &Elision{}

// Scalar is the type used to represent all scalar JSON values, i.e.
// - strings
// - numbers
// - booleans (to values)
// - null (a single value)
//
// The type is encoded in the Type field, while the Bytes fields contains the
// literal representation of the value as found in the input.
type Scalar struct {

	// Literal representation of the value, e.g.
	// - the string "foo" is represented as []byte("\"foo\"")
	// - the number 123.5 is represented as []byte("132.5")
	// - the boolean true is represented as []byte("true")
	Bytes []byte

	// Type of the value
	TypeAndFlags uint8
}

// EqualsString is a convenience method to check if a Scalar represents the
// passed string.
//
// TODO move that somewhere more suitable.
func (s *Scalar) EqualsString(str string) bool {
	if s.Type() != String {
		return false
	}
	return string(s.Bytes[1:len(s.Bytes)-1]) == str
}

func NewScalar(tp ScalarType, bytes []byte) *Scalar {
	return &Scalar{
		Bytes:        bytes,
		TypeAndFlags: uint8(tp),
	}
}

func NewKey(tp ScalarType, bytes []byte) *Scalar {
	return &Scalar{
		Bytes:        bytes,
		TypeAndFlags: uint8(tp) | KeyMask,
	}
}
func (s *Scalar) Type() ScalarType {
	return (ScalarType(s.TypeAndFlags & TypeMask))
}

func (s *Scalar) IsKey() bool {
	return KeyMask&s.TypeAndFlags != 0
}

func (s *Scalar) IsAlnum() bool {
	return AlnumMask&s.TypeAndFlags != 0
}

func (s *Scalar) IsUnescaped() bool {
	return UnescapedMask&s.TypeAndFlags != 0
}

func (s *Scalar) String() string {
	return fmt.Sprintf("Scalar(%s)", s.Bytes)
}

func (s *Scalar) Equals(t *Scalar) bool {
	if s == nil || t == nil {
		return false
	}
	if s.Type() != t.Type() {
		return false
	}
	switch s.Type() {
	case String, Number, Boolean:
		return bytes.Equal(s.Bytes, t.Bytes)
	case Null:
		return true
	default:
		panic("invalid scalar type")
	}
}

// ScalarType encodes the four possible JSON scalar types.
type ScalarType uint8

const (
	Null               = 0x0 // the type of JSON null
	Boolean            = 0x1 // a JSON boolean
	Number             = 0x2 // a JSON number
	String  ScalarType = 0x3 // a JSON string
)

const (
	TypeMask      = 0b00011
	KeyMask       = 0b00100
	AlnumMask     = 0b01000
	UnescapedMask = 0b10000
)
