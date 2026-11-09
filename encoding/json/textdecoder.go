package json

import (
	"bufio"
	"bytes"
	"encoding/json/jsontext"
	"fmt"
	"io"

	"github.com/arnodel/jsonstream/internal/jsontoken"
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
	return &textDecoder{lines: lines, dec: jsontext.NewDecoder(lines, jsontoken.Options...)}
}

func (d *textDecoder) produce(out token.WriteStream) error {
	if err := jsontoken.Put(d.dec, out, false); err != nil {
		return d.syntaxError(err)
	}
	return nil
}

// A SyntaxError is invalid input to a Decoder, or to the decoder of another
// format that contains JSON.
type SyntaxError struct {
	Line, Column int64  // 1-based position of the error in bytes, 0 if unknown
	Offset       int64  // byte offset of the error
	Msg          string // what is wrong
	Err          error  // the error it comes from, if any; the end of the input inside a value wraps io.ErrUnexpectedEOF
}

func (e *SyntaxError) Error() string {
	if e.Line == 0 {
		return fmt.Sprintf("syntax error at offset %d: %s", e.Offset, e.Msg)
	}
	return fmt.Sprintf("syntax error at L%d,C%d: %s", e.Line, e.Column, e.Msg)
}

func (e *SyntaxError) Unwrap() error {
	return e.Err
}

// syntaxError turns a jsontext error into a *SyntaxError.
func (d *textDecoder) syntaxError(err error) error {
	msg, ptr, offset, ok := jsontoken.DescribeError(err)
	if !ok {
		return err
	}
	if ptr != "" {
		msg += " within " + string(ptr)
	}
	line, col, _ := d.lines.position(offset)
	return &SyntaxError{Line: line, Column: col, Offset: offset, Msg: msg, Err: err}
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
