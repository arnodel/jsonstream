package jsonstream

import (
	"bufio"
	"errors"
	"fmt"
	"io"
)

// JPVDecoder reads input in JPV format and streams it into a JSON stream.
//
// JPV is a format that can represent values where each line specifies a leaf
// value and its path.  A Lines are separated by '\n' and are of the form
//
//	<path> = <value>
//
// where <path> is a JSONPath and <value> is a JSON value.  E.g.
//
//	{"name": "Dan", "parent_ids": [132, 7650]}
//
// is represented as
//
//	$.["name"] = "Dan"
//	$.["parent_ids"][0] = 132
//	$.["parent_ids"][1] = 7650
//
// The potential value in this format is that it can be piped through grep and
// other unix utilites to be filtered / transformed, then turned back into JSON.
type JPVDecoder struct {
	buf      *bufio.Reader
	lastPath []*Scalar
}

var _ StreamSource = &JPVDecoder{}

// NewJPVDecoder sets up a new GRONDecoder instance to read from the given
// input.
func NewJPVDecoder(in io.Reader) *JPVDecoder {
	return &JPVDecoder{buf: bufio.NewReader(in)}
}

// Produce reads a stream of JPV values and streams them, until it runs out of
// input or encounters invalid JPV, in which case it will return an error.
func (d *JPVDecoder) Produce(out chan<- StreamItem) error {
	for {
		err := d.parseLine(out)
		if err != nil {
			if errors.Is(err, io.EOF) {
				unwindPath(d.lastPath, false, out)
				return nil
			}
			return err
		}
	}
}

func (d *JPVDecoder) parseLine(out chan<- StreamItem) error {
	b, err := skipSpace(d.buf)
	if err != nil {
		return err
	}
	if b != '$' {
		return fmt.Errorf("syntax error: expected '$', got %q", b)
	}
	linePath, err := parsePath(d.buf)
	if err != nil {
		return err
	}
	b, err = skipSpace(d.buf)
	if err != nil {
		return err
	}
	if b != '=' {
		return fmt.Errorf("syntax error: expected '=', got %q", b)
	}
	err = d.updatePath(linePath, out)
	if err != nil {
		return err
	}
	// TODO: tidy this up
	jsonDecoder := JSONDecoder{buf: d.buf}
	return jsonDecoder.parseValue(out)
}

func (d *JPVDecoder) updatePath(newPath []*Scalar, out chan<- StreamItem) error {
	if len(d.lastPath) == 0 {
		followPath(newPath, false, out)
		d.lastPath = newPath
		return nil
	}
	divergenceIndex := -1
	for i, key := range d.lastPath {
		newKey := newPath[i]
		if i >= len(newPath) {
			return errors.New("inconsistent path: cannot be a prefix of the previous path")
		}
		if !key.Equals(newKey) {
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
	unwindPath(d.lastPath[divergenceIndex:], true, out)

	// Open object and arrays the new object is in
	followPath(newPath[divergenceIndex:], true, out)
	d.lastPath = newPath
	return nil
}

func unwindPath(path []*Scalar, inCollection bool, out chan<- StreamItem) {
	for i := len(path) - 1; i >= 0; i-- {
		if i > 0 || !inCollection {
			switch path[i].Type() {
			case String:
				out <- &EndObject{}
			case Number:
				out <- &EndArray{}
			default:
				panic("invalid key type (must be string or number)")
			}
		}
	}
}

func followPath(path []*Scalar, inCollection bool, out chan<- StreamItem) {
	for _, key := range path {
		switch key.Type() {
		case String:
			if !inCollection {
				out <- &StartObject{}
			}
			out <- key
		case Number:
			if !inCollection {
				out <- &StartArray{}
			}
		default:
			panic("invalid key type (must be string or number)")
		}
		inCollection = false
	}
}

func parsePath(buf *bufio.Reader) ([]*Scalar, error) {
	var path []*Scalar
	for {
		b, err := buf.ReadByte()
		if err != nil {
			// That's ok because paths are followed by a value
			return nil, err
		}
		if b != '[' {
			buf.UnreadByte()
			return path, nil
		}
		if b == '[' {
			b, err = buf.ReadByte()
			if err != nil {
				return nil, err
			}
			if b == '"' {
				s, err := parseString(buf)
				if err != nil {
					return nil, err
				}
				s.TypeAndFlags |= KeyMask
				path = append(path, s)
				b, err = buf.ReadByte()
				if err != nil {
					return nil, err
				}
			} else {
				var intBytes []byte
				var n int
				b, n, err = readDigits(buf, b, &intBytes)
				if err != nil {
					return nil, err
				}
				if n == 0 {
					return nil, errors.New("syntax error: expected integer")
				}
				path = append(path, NewKey(Number, intBytes))
			}
			if b != ']' {
				return nil, errors.New("syntax error: expected ']'")
			}
		}
	}
}

func isalpha[T byte | rune](b T) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b == '_'
}

func isdigit[T byte | rune](b T) bool {
	return b >= '0' && b <= '9'
}

func isalnum[T byte | rune](b T) bool {
	return isalpha(b) || isdigit(b)
}

func isctrl[T byte | rune](b T) bool {
	return b < 32
}
