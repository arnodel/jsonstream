package token

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
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
type Token interface {
	fmt.Stringer
}

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
func (s *Scalar) EqualsString(str string) bool {
	if s.Type() != String {
		return false
	}
	return s.ToString() == str
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

func (s *Scalar) Equal(t *Scalar) bool {
	if s == nil || t == nil {
		return false
	}
	if s.Type() != t.Type() {
		return false
	}
	switch s.Type() {
	case Null:
		return true
	case Boolean:
		// The bytes are "true" or "false", so it's enough to compare the first one
		return s.Bytes[0] == t.Bytes[0]
	case String:
		if bytes.Equal(s.Bytes, t.Bytes) {
			return true
		}
		if s.IsUnescaped() && t.IsUnescaped() {
			return false
		}
	case Number:
		if bytes.Equal(s.Bytes, t.Bytes) {
			return true
		}
	default:
		panic("invalid scalar type")
	}
	// Fall back to slower conversion
	return parseJsonLiteralBytes(s.Bytes) == parseJsonLiteralBytes(t.Bytes)
}

// panics if not a string
func (s *Scalar) ToString() string {
	if s.IsUnescaped() {
		return string(s.Bytes[1 : len(s.Bytes)-1])
	}
	return parseJsonLiteralBytes(s.Bytes).(string)
}

func (s *Scalar) ToGo() any {
	if s.IsUnescaped() {
		return string(s.Bytes[1 : len(s.Bytes)-1])
	}
	return parseJsonLiteralBytes(s.Bytes)
}

func parseJsonLiteralBytes(b []byte) json.Token {
	dec := json.NewDecoder(bytes.NewReader(b))
	tok, err := dec.Token()
	if err != nil {
		panic(err)
	}
	return tok
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

var (
	trueBytes  = []byte("true")
	falseBytes = []byte("false")
	nullBytes  = []byte("null")
)

var (
	TrueScalar  = NewScalar(Boolean, trueBytes)
	FalseScalar = NewScalar(Boolean, falseBytes)
	NullScalar  = NewScalar(Null, nullBytes)
)

func StringScalar(s string) *Scalar {
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	if err := encoder.Encode(s); err != nil {
		panic(err)
	}
	var encodedBytes = b.Bytes()
	// Remove the new line at the end
	return NewScalar(String, encodedBytes[:len(encodedBytes)-1])
}

func Float64Scalar(x float64) *Scalar {
	return NewScalar(Number, []byte(strconv.FormatFloat(x, 'e', -1, 64)))
}

func Int64Scalar(n int64) *Scalar {
	return NewScalar(Number, []byte(strconv.FormatInt(n, 10)))
}

func BoolScalar(b bool) *Scalar {
	if b {
		return TrueScalar
	}
	return FalseScalar
}

func ToScalar(value any) (*Scalar, error) {
	if value == nil {
		return NullScalar, nil
	}
	switch x := value.(type) {
	case string:
		return StringScalar(x), nil
	case float64:
		return Float64Scalar(x), nil
	case int64:
		return Int64Scalar(x), nil
	case int:
		return Int64Scalar(int64(x)), nil
	case bool:
		return BoolScalar(x), nil
	default:
		return nil, errors.New("not a scalar value")
	}
}
