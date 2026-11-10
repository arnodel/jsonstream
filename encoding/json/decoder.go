package json

import (
	"bufio"
	"bytes"
	"encoding/json/jsontext"
	"fmt"
	"io"
	"strings"

	"github.com/arnodel/jsonstream/internal/jsontoken"
	"github.com/arnodel/jsonstream/token"
)

// A Decoder reads JSON input and streams it into a JSON stream.  It
// tokenises the input with encoding/json/jsontext, which also checks the
// syntax.
type Decoder struct {
	dec   *jsontext.Decoder
	lines *lineCounter
	in    *token.FlushingReader
}

var _ token.StreamSource = &Decoder{}

// NewDecoder sets up a new Decoder instance to read from the given input.
func NewDecoder(in io.Reader) *Decoder {
	fr := &token.FlushingReader{Reader: in}
	// jsontext reads 4KB at a time, and each read of a FlushingReader sends
	// on a partial batch of tokens.
	lines := &lineCounter{r: bufio.NewReaderSize(fr, 64<<10)}
	return &Decoder{dec: jsontext.NewDecoder(lines, jsontoken.Options...), lines: lines, in: fr}
}

// Produce reads a stream of JSON values and streams them, until it runs out
// of input or encounters invalid JSON, in which case it returns a
// *SyntaxError.
func (d *Decoder) Produce(out token.WriteStream) error {
	// Send the tokens produced so far before blocking on input.
	d.in.Flush = token.FlushFunc(out)
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
func (d *Decoder) syntaxError(err error) error {
	msg, ptr, offset, ok := jsontoken.DescribeError(err)
	if !ok {
		return err
	}
	if ptr != "" {
		msg += " within " + shortPointer(string(ptr))
	}
	line, col, _ := d.lines.position(offset)
	return &SyntaxError{Line: line, Column: col, Offset: offset, Msg: msg, Err: err}
}

// pointerEnds is how many names shortPointer keeps at each end of a pointer.
const pointerEnds = 4

// shortPointer is the JSON Pointer ptr with its middle names left out if it
// has many, saying how deep it goes instead: an error deep in a document would
// otherwise repeat thousands of names.  A name cannot contain '/', which is
// escaped as ~1, so splitting on it is safe.
func shortPointer(ptr string) string {
	names := strings.Split(ptr, "/")[1:]
	if len(names) <= 2*pointerEnds+1 {
		return ptr
	}
	return fmt.Sprintf("/%s/.../%s (%d levels deep)",
		strings.Join(names[:pointerEnds], "/"),
		strings.Join(names[len(names)-pointerEnds:], "/"),
		len(names))
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
