// Package jsontoken turns the output of a jsontext.Decoder into tokens.  It
// is shared by the decoders of the formats that contain JSON values.
package jsontoken

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"io"

	"github.com/arnodel/jsonstream/internal/scanner"
	"github.com/arnodel/jsonstream/token"
)

// Options make a jsontext.Decoder accept what the JSON grammar accepts: it is
// stricter by default.
var Options = []jsontext.Options{
	jsontext.AllowDuplicateNames(true),
	jsontext.AllowInvalidUTF8(true),
}

// Put reads JSON values from dec and puts their tokens to out: one value if
// one is true, otherwise all of them until the end of the input.  Errors are
// those of dec, and io.EOF if one is true and there is no value.
func Put(dec *jsontext.Decoder, out token.WriteStream, one bool) error {
	for {
		var tok token.Token
		switch dec.PeekKind() {
		case jsontext.KindBeginObject:
			tok = &token.StartObject{}
		case jsontext.KindEndObject:
			tok = &token.EndObject{}
		case jsontext.KindBeginArray:
			tok = &token.StartArray{}
		case jsontext.KindEndArray:
			tok = &token.EndArray{}
		case jsontext.KindNull:
			tok = nullInstance
		case jsontext.KindTrue:
			tok = trueInstance
		case jsontext.KindFalse:
			tok = falseInstance
		case jsontext.KindString:
			v, err := dec.ReadValue()
			if err != nil {
				return err
			}
			s := StringScalar(v)
			// Once a name is read its object has an odd number of tokens.
			if kind, n := dec.StackIndex(dec.StackDepth()); kind == jsontext.KindBeginObject && n%2 == 1 {
				s.TypeAndFlags |= token.KeyMask
			}
			out.Put(s)
			if one && dec.StackDepth() == 0 {
				return nil
			}
			continue
		case jsontext.KindNumber:
			v, err := dec.ReadValue()
			if err != nil {
				return err
			}
			out.Put(token.CopyScalar(token.Number, v))
			if one && dec.StackDepth() == 0 {
				return nil
			}
			continue
		default:
			// An error, or the end of the input.
			_, err := dec.ReadToken()
			if err == io.EOF && !one {
				return nil
			}
			return err
		}
		if _, err := dec.ReadToken(); err != nil {
			return err
		}
		out.Put(tok)
		if one && dec.StackDepth() == 0 {
			return nil
		}
	}
}

// StringScalar makes a string scalar from the literal v, which is only valid
// until the next read.
func StringScalar(v jsontext.Value) *token.Scalar {
	s := token.CopyScalar(token.String, v)
	body := s.Bytes[1 : len(s.Bytes)-1]
	if bytes.IndexByte(body, '\\') < 0 {
		s.TypeAndFlags |= token.UnescapedMask
		if IsName(body) {
			s.TypeAndFlags |= token.AlnumMask
		}
	}
	return s
}

// IsName reports whether b can be written as a shorthand name in a path.
func IsName(b []byte) bool {
	if len(b) == 0 || !scanner.IsAlpha(b[0]) {
		return false
	}
	for _, c := range b[1:] {
		if !scanner.IsAlnum(c) {
			return false
		}
	}
	return true
}

var (
	trueInstance  = token.NewScalar(token.Boolean, []byte("true"))
	falseInstance = token.NewScalar(token.Boolean, []byte("false"))
	nullInstance  = token.NewScalar(token.Null, []byte("null"))
)

// DescribeError returns what a syntax error reported by jsontext is, the
// JSON Pointer of the value it is in, and its byte offset.  ok is false if
// err is not a syntax error.
func DescribeError(err error) (msg string, ptr jsontext.Pointer, offset int64, ok bool) {
	var serr *jsontext.SyntacticError
	if !errors.As(err, &serr) {
		return "", "", 0, false
	}
	msg = serr.Err.Error()
	if errors.Is(serr.Err, io.ErrUnexpectedEOF) {
		msg = "unexpected end of input"
	}
	return msg, serr.JSONPointer, serr.ByteOffset, true
}
