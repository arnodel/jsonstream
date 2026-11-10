package json

import (
	"bufio"
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"

	"github.com/arnodel/jsonstream/internal/scanner"
	"github.com/arnodel/jsonstream/token"
)

// textDecoder turns the output of a jsontext.Decoder, which does the
// tokenising and checks the syntax, into a token stream.
type textDecoder struct {
	dec   *jsontext.Decoder
	lines *lineCounter
}

func newTextDecoder(in io.Reader) *textDecoder {
	// jsontext reads 4KB at a time, and each read of a FlushingReader sends
	// on a partial batch of tokens.
	lines := &lineCounter{r: bufio.NewReaderSize(in, 64<<10)}
	return &textDecoder{lines: lines, dec: jsontext.NewDecoder(lines,
		// Accept what the JSON grammar accepts: jsontext is stricter by
		// default.
		jsontext.AllowDuplicateNames(true),
		jsontext.AllowInvalidUTF8(true),
	)}
}

func (d *textDecoder) produce(out token.WriteStream) error {
	if err := d.produceTokens(out); err != nil {
		return d.syntaxError(err)
	}
	return nil
}

func (d *textDecoder) produceTokens(out token.WriteStream) error {
	dec := d.dec
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
			s := stringScalar(v)
			// Once a name is read its object has an odd number of tokens.
			if kind, n := dec.StackIndex(dec.StackDepth()); kind == jsontext.KindBeginObject && n%2 == 1 {
				s.TypeAndFlags |= token.KeyMask
			}
			out.Put(s)
			continue
		case jsontext.KindNumber:
			v, err := dec.ReadValue()
			if err != nil {
				return err
			}
			out.Put(token.CopyScalar(token.Number, v))
			continue
		default:
			// An error, or the end of the input.
			_, err := dec.ReadToken()
			if err == io.EOF {
				return nil
			}
			return err
		}
		if _, err := dec.ReadToken(); err != nil {
			return err
		}
		out.Put(tok)
	}
}

// stringScalar makes a string scalar from the literal v, which is only valid
// until the next read.
func stringScalar(v jsontext.Value) *token.Scalar {
	s := token.CopyScalar(token.String, v)
	body := s.Bytes[1 : len(s.Bytes)-1]
	if bytes.IndexByte(body, '\\') < 0 {
		s.TypeAndFlags |= token.UnescapedMask
		if isName(body) {
			s.TypeAndFlags |= token.AlnumMask
		}
	}
	return s
}

// isName reports whether b can be written as a shorthand name in a path.
func isName(b []byte) bool {
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

// A SyntaxError is invalid JSON in the input of a Decoder.  It wraps the
// error reported by jsontext; the end of the input inside a value wraps
// io.ErrUnexpectedEOF.
type SyntaxError struct {
	Line, Column int64 // 1-based position of the error in bytes, 0 if unknown
	Offset       int64 // byte offset of the error
	msg          string
	err          error
}

func (e *SyntaxError) Error() string {
	if e.Line == 0 {
		return fmt.Sprintf("syntax error at offset %d: %s", e.Offset, e.msg)
	}
	return fmt.Sprintf("syntax error at L%d,C%d: %s", e.Line, e.Column, e.msg)
}

func (e *SyntaxError) Unwrap() error {
	return e.err
}

// syntaxError turns a jsontext error into a *SyntaxError.
func (d *textDecoder) syntaxError(err error) error {
	var serr *jsontext.SyntacticError
	if !errors.As(err, &serr) {
		return err
	}
	msg := serr.Err.Error()
	if errors.Is(serr.Err, io.ErrUnexpectedEOF) {
		msg = "unexpected end of input"
	}
	if serr.JSONPointer != "" {
		msg += " within " + string(serr.JSONPointer)
	}
	line, col, _ := d.lines.position(serr.ByteOffset)
	return &SyntaxError{Line: line, Column: col, Offset: serr.ByteOffset, msg: msg, err: serr}
}

// lineCounter passes input through and remembers where its newlines are, so
// that the byte offset of an error can be turned into a line and column.  It
// keeps the offsets of the newlines in the last lineWindow bytes read: the
// decoder reads ahead, but never by that much except inside a huge value.
type lineCounter struct {
	r        io.Reader
	n        int64   // bytes read so far
	lines    int64   // newlines read so far
	recent   []int64 // offsets of the newlines in the window, in order
	lastGone int64   // offset of the last newline dropped from recent, or -1
}

const lineWindow = 1 << 20

func (c *lineCounter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	for i, b := 0, p[:n]; ; {
		j := bytes.IndexByte(b[i:], '\n')
		if j < 0 {
			break
		}
		c.recent = append(c.recent, c.n+int64(i+j))
		c.lines++
		i += j + 1
	}
	c.n += int64(n)
	if len(c.recent) > 0 && c.recent[0] < c.n-lineWindow {
		k := 0
		for k < len(c.recent) && c.recent[k] < c.n-lineWindow {
			k++
		}
		c.lastGone = c.recent[k-1]
		c.recent = append(c.recent[:0], c.recent[k:]...)
	}
	return n, err
}

// position returns the 1-based line and column of the byte at offset, or
// false if offset is too far behind what was read.
func (c *lineCounter) position(offset int64) (line, col int64, ok bool) {
	if offset < c.n-lineWindow {
		return 0, 0, false
	}
	// The newlines at or after offset are all in recent.
	after := 0
	for i := len(c.recent) - 1; i >= 0 && c.recent[i] >= offset; i-- {
		after++
	}
	lineStart := int64(0)
	if k := len(c.recent) - after; k > 0 {
		lineStart = c.recent[k-1] + 1
	} else if c.lines > int64(after) {
		lineStart = c.lastGone + 1
	}
	return c.lines - int64(after) + 1, offset - lineStart + 1, true
}
