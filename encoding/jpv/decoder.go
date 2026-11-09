package jpv

import (
	"bufio"
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"

	"github.com/arnodel/jsonstream/encoding/json"
	"github.com/arnodel/jsonstream/internal/jsontoken"
	"github.com/arnodel/jsonstream/internal/scanner"
	"github.com/arnodel/jsonstream/token"
)

// Decoder reads input in JPV format and streams it into a JSON stream.
//
// JPV is a format that can represent values where each line specifies a leaf
// value and its path.  Lines are separated by '\n' and are of the form
//
//	<path> = <value>
//
// where <path> is a JSONPath and <value> is a JSON value.  E.g.
//
//	{"name": "Dan", "parent_ids": [132, 7650]}
//
// is represented as
//
//	$.name = "Dan"
//	$.parent_ids[0] = 132
//	$.parent_ids[1] = 7650
//
// The potential value in this format is that it can be piped through grep and
// other unix utilites to be filtered / transformed, then turned back into JSON.
//
// A value can continue on the lines that follow, but nothing else can come
// after it on the line where it ends.
type Decoder struct {
	in    *token.FlushingReader
	src   lineSource
	value *jsontext.Decoder // reads the values from src
	paths pathTracker
}

var _ token.StreamSource = &Decoder{}

// NewDecoder sets up a new Decoder instance to read from the given
// input.
func NewDecoder(in io.Reader) *Decoder {
	fr := &token.FlushingReader{Reader: in}
	d := &Decoder{in: fr}
	d.src.r = bufio.NewReaderSize(fr, 64<<10)
	d.value = jsontext.NewDecoder(&d.src, jsontoken.Options...)
	return d
}

// Produce reads a stream of JPV values and streams them, until it runs out of
// input or encounters invalid JPV, in which case it will return an error.
func (d *Decoder) Produce(out token.WriteStream) error {
	// Send the tokens produced so far before blocking on input.
	d.in.Flush = token.FlushFunc(out)
	for {
		line := d.src.nextLine()
		if len(line) > 0 {
			if err := d.parseLine(line, out); err != nil {
				// Leave what is open unclosed: the stream ends inside a value.
				return err
			}
		}
		if err := d.src.err; err != nil {
			if err == io.EOF {
				d.paths.close(out)
				return nil
			}
			return err
		}
	}
}

// parseLine parses the entry that starts on line, which can be blank.
func (d *Decoder) parseLine(line []byte, out token.WriteStream) error {
	i := skipSpace(line, 0)
	if i == len(line) || line[i] == '\n' {
		return nil
	}
	if line[i] != '$' {
		return d.lineError(line, i, "expected '$', got %s", describeByte(line, i))
	}
	path, i, err := d.parsePath(line, i+1)
	if err != nil {
		return err
	}
	i = skipSpace(line, i)
	if i == len(line) || line[i] == '\n' {
		err := d.lineError(line, i, "expected '=', got %s", describeByte(line, i))
		err.Err = errLineEndsBeforeEquals
		return err
	}
	if line[i] != '=' {
		return d.lineError(line, i, "expected '=', got %s", describeByte(line, i))
	}
	if err := d.paths.update(path, out); err != nil {
		return err
	}
	return d.parseValue(line, i+1, out)
}

// parseValue parses the value that starts at line[i:] and may continue on
// the next lines.
func (d *Decoder) parseValue(line []byte, i int, out token.WriteStream) error {
	d.src.startValue(line, i)
	d.value.Reset(&d.src, jsontoken.Options...)
	err := jsontoken.Put(d.value, out, true)
	if err == io.EOF {
		return d.valueError("unexpected end of input, expected a value", d.value.InputOffset(), io.ErrUnexpectedEOF)
	}
	if msg, _, offset, ok := jsontoken.DescribeError(err); ok {
		return d.valueError(msg, offset, err)
	}
	if err != nil {
		return err
	}
	// The value has ended: what the decoder has read beyond it, and the rest
	// of the line, must be blank.
	if !isBlank(d.value.UnreadBuffer()) || !isBlank(d.src.pending) {
		return d.valueError("expected the end of the line after the value", d.value.InputOffset(), errMoreAfterValue)
	}
	return nil
}

// parsePath parses the path that starts at line[i:], after the '$', and
// returns it with the index of the byte after it.
func (d *Decoder) parsePath(line []byte, i int) ([]*token.Scalar, int, error) {
	var path []*token.Scalar
	for i < len(line) {
		switch line[i] {
		case '[':
			i++
			var key *token.Scalar
			if i < len(line) && line[i] == '"' {
				end := stringEnd(line, i)
				if end < 0 || !jsontext.Value(line[i:end]).IsValid(jsontoken.Options...) {
					return nil, 0, d.lineError(line, i, "invalid string in path")
				}
				key = jsontoken.StringScalar(line[i:end])
				key.TypeAndFlags |= token.KeyMask
				i = end
			} else {
				j := i
				for j < len(line) && scanner.IsDigit(line[j]) {
					j++
				}
				if j == i {
					return nil, 0, d.lineError(line, i, "expected digit, got %s", describeByte(line, i))
				}
				key = token.NewKey(token.Number, bytes.Clone(line[i:j]))
				i = j
			}
			if i == len(line) || line[i] != ']' {
				return nil, 0, d.lineError(line, i, "expected ']', got %s", describeByte(line, i))
			}
			i++
			path = append(path, key)
		case '.':
			i++
			j := i
			if j < len(line) && scanner.IsAlpha(line[j]) {
				j++
				for j < len(line) && scanner.IsAlnum(line[j]) {
					j++
				}
			}
			if j == i {
				return nil, 0, d.lineError(line, i, "expected a-z/A-Z/_, got %s", describeByte(line, i))
			}
			key := token.NewKey(token.String, append(append([]byte{'"'}, line[i:j]...), '"'))
			key.TypeAndFlags |= token.AlnumMask | token.UnescapedMask
			path = append(path, key)
			i = j
		default:
			return path, i, nil
		}
	}
	return path, i, nil
}

// Causes of the errors for what the decoder rejected before it used jsontext.
var (
	errLineEndsBeforeEquals = errors.New("the line ends before '='")
	errMoreAfterValue       = errors.New("the line goes on after the value")
)

// lineError is a syntax error at line[i], line being the current line.
func (d *Decoder) lineError(line []byte, i int, format string, args ...any) *json.SyntaxError {
	return &json.SyntaxError{
		Line:   d.src.lineNo,
		Column: int64(i) + 1,
		Offset: d.src.lineStart + int64(i),
		Msg:    fmt.Sprintf(format, args...),
	}
}

// valueError is a syntax error at offset in the value being parsed.
func (d *Decoder) valueError(msg string, offset int64, err error) error {
	line, col, offset := d.src.valuePosition(offset)
	return &json.SyntaxError{Line: line, Column: col, Offset: offset, Msg: msg, Err: err}
}

// describeByte describes line[i] for an error message.
func describeByte(line []byte, i int) string {
	switch {
	case i == len(line):
		return "end of input"
	case line[i] == '\n' || line[i] == '\r':
		return "end of line"
	case line[i] >= 0x80:
		// Not a character on its own: %q would show it as the code point.
		return fmt.Sprintf(`'\x%02x'`, line[i])
	default:
		return fmt.Sprintf("%q", line[i])
	}
}

// stringEnd returns the index after the end of the string literal that
// starts at line[i], or -1 if it does not end on the line.
func stringEnd(line []byte, i int) int {
	for j := i + 1; j < len(line); j++ {
		switch line[j] {
		case '\\':
			j++
		case '"':
			return j + 1
		case '\n':
			return -1
		}
	}
	return -1
}

// skipSpace returns the index of the first byte from line[i] on that is not a
// space, tab or carriage return.
func skipSpace(line []byte, i int) int {
	for i < len(line) && (line[i] == ' ' || line[i] == '\t' || line[i] == '\r') {
		i++
	}
	return i
}

func isBlank(b []byte) bool {
	for _, c := range b {
		if c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			return false
		}
	}
	return true
}

// A lineSource reads the input a line at a time.  The decoder parses the path
// of an entry from its line, then a jsontext decoder reads the value from the
// rest of the line through Read, which gives it the following lines only as
// it asks for them: so a value can span lines, but the jsontext decoder never
// reads past the line it ends on.
type lineSource struct {
	r         *bufio.Reader
	err       error  // the error that ended the input, io.EOF at its end
	long      []byte // holds lines longer than r's buffer
	lineNo    int64  // 1-based number of the current line
	lineStart int64  // input offset of the current line
	next      int64  // input offset of the next line
	lastByte  byte   // the last byte of the current line

	pending    []byte  // the rest of the current line, not yet read by Read
	valueStart int64   // input offset of the start of the value being read
	valueLines []int64 // input offsets of the lines the value is on
}

// nextLine returns the next line, with its '\n' if it has one, or nil at the
// end of the input; s.err is then set.  The line is valid until the next
// call.
func (s *lineSource) nextLine() []byte {
	if s.err != nil {
		return nil
	}
	line, err := s.r.ReadSlice('\n')
	if err == bufio.ErrBufferFull {
		s.long = append(s.long[:0], line...)
		for err == bufio.ErrBufferFull {
			line, err = s.r.ReadSlice('\n')
			s.long = append(s.long, line...)
		}
		line = s.long
	}
	s.err = err
	if len(line) == 0 {
		return nil
	}
	s.lastByte = line[len(line)-1]
	s.lineNo++
	s.lineStart = s.next
	s.next += int64(len(line))
	return line
}

// startValue sets up s for reading a value from line[i:] on.
func (s *lineSource) startValue(line []byte, i int) {
	s.pending = line[i:]
	s.valueStart = s.lineStart + int64(i)
	s.valueLines = append(s.valueLines[:0], s.lineStart)
}

// Read reads the rest of the current line, then the next lines, one per call.
func (s *lineSource) Read(p []byte) (int, error) {
	for len(s.pending) == 0 {
		s.pending = s.nextLine()
		if s.pending == nil {
			return 0, s.err
		}
		s.valueLines = append(s.valueLines, s.lineStart)
	}
	n := copy(p, s.pending)
	s.pending = s.pending[n:]
	return n, nil
}

// valuePosition returns the line, column and input offset of the byte at
// offset in the value being read.
func (s *lineSource) valuePosition(offset int64) (line, col, inputOffset int64) {
	inputOffset = s.valueStart + offset
	if inputOffset == s.next && s.next > s.lineStart && s.lastByte == '\n' {
		// Past the end of a last line that ends with a newline: the error is
		// at the start of the line after it.
		return s.lineNo + 1, 1, inputOffset
	}
	k := len(s.valueLines) - 1
	for k > 0 && s.valueLines[k] > inputOffset {
		k--
	}
	first := s.lineNo - int64(len(s.valueLines)-1)
	return first + int64(k), inputOffset - s.valueLines[k] + 1, inputOffset
}

// A pathTracker turns the path of each entry into the tokens that open and
// close the arrays and objects around its value.
type pathTracker struct {
	last []*token.Scalar
}

// update puts the tokens that lead from the previous path to newPath.
func (t *pathTracker) update(newPath []*token.Scalar, out token.WriteStream) error {
	if len(t.last) == 0 {
		followPath(newPath, false, out)
		t.last = newPath
		return nil
	}
	divergenceIndex := -1
	for i, key := range t.last {
		if i >= len(newPath) {
			return errors.New("inconsistent path: cannot be a prefix of the previous path")
		}
		newKey := newPath[i]
		if !key.Equal(newKey) {
			if key.Type() != newKey.Type() {
				return errors.New("inconsistent path: key types differ")
			}
			divergenceIndex = i
			break
		}
	}
	if divergenceIndex == -1 {
		return errors.New("inconsistent path: cannot extend previous path")
	}

	// Close objects an arrays that we are no longer in
	unwindPath(t.last[divergenceIndex:], true, out)

	// Open object and arrays the new object is in
	followPath(newPath[divergenceIndex:], true, out)
	t.last = newPath
	return nil
}

// close puts the tokens that close the arrays and objects of the last path.
func (t *pathTracker) close(out token.WriteStream) {
	unwindPath(t.last, false, out)
}

func unwindPath(path []*token.Scalar, inCollection bool, out token.WriteStream) {
	for i := len(path) - 1; i >= 0; i-- {
		if i > 0 || !inCollection {
			switch path[i].Type() {
			case token.String:
				out.Put(&token.EndObject{})
			case token.Number:
				out.Put(&token.EndArray{})
			default:
				panic("invalid key type (must be string or number)")
			}
		}
	}
}

func followPath(path []*token.Scalar, inCollection bool, out token.WriteStream) {
	for _, key := range path {
		switch key.Type() {
		case token.String:
			if !inCollection {
				out.Put(&token.StartObject{})
			}
			out.Put(key)
		case token.Number:
			if !inCollection {
				out.Put(&token.StartArray{})
			}
		default:
			panic("invalid key type (must be string or number)")
		}
		inCollection = false
	}
}
