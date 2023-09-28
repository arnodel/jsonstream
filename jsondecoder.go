package jsonstream

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
)

// A JSONDecoder reads JSON input and streams it into a JSON stream.
type JSONDecoder struct {
	buf *bufio.Reader
}

var _ StreamSource = &JSONDecoder{}

// NewJSONDecoder sets up a new JSONDecoder instance to read from the giver input.
func NewJSONDecoder(in io.Reader) *JSONDecoder {
	return &JSONDecoder{buf: bufio.NewReader(in)}
}

// Produce reads a stream of JSON values and streams them, until it runs
// out of input or encounter invalid JSON, in which case it will return an
// error.
func (r *JSONDecoder) Produce(out chan<- StreamItem) error {
	for {
		err := r.parseValue(out)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// parseValue reads a single JSON value and streams it.  It can return a
// non-nil error if the input is invalid JSON.
func (r *JSONDecoder) parseValue(out chan<- StreamItem) error {
	b, err := skipSpace(r.buf)
	if err != nil {
		return err
	}
	return r.parseValueFirstByte(out, b)
}

func (r *JSONDecoder) parseValueFirstByte(out chan<- StreamItem, b byte) error {
	var err error
	switch b {
	case '"':
		return r.parseString(out)
	case '[':
		err = r.parseArray(out)
	case '{':
		err = r.parseObject(out)
	case 't':
		return r.parseTrue(out)
	case 'f':
		return r.parseFalse(out)
	case 'n':
		return r.parseNull(out)
	default:
		if b == '-' || b >= '0' && b <= '9' {
			return r.parseNumber(b, out)
		}
		err = fmt.Errorf("syntax error: invalid value starting with %q", b)
	}
	return err
}

// The leading '"" has already been consumed
func (r *JSONDecoder) parseString(out chan<- StreamItem) error {
	s, err := parseString(r.buf)
	if err != nil {
		return err
	}
	out <- s
	return nil
}

// The leading '"" has already been consumed
func (r *JSONDecoder) parseKey(out chan<- StreamItem) error {
	s, err := parseString(r.buf)
	if err != nil {
		return err
	}
	s.TypeAndFlags |= KeyMask
	out <- s
	return nil
}

func (r *JSONDecoder) parseArray(out chan<- StreamItem) error {
	out <- &StartArray{}
	b, err := skipSpace(r.buf)
	if err != nil {
		return err
	}
	if b == ']' {
		out <- &EndArray{}
		return nil
	}
	for {
		err := r.parseValueFirstByte(out, b)
		if err != nil {
			return err
		}
		b, err = skipSpace(r.buf)
		if err != nil {
			return err
		}
		switch b {
		case ']':
			out <- &EndArray{}
			return nil
		case ',':
			b, err = skipSpace(r.buf)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("syntax error: expected ']' or ',', got %q", []byte{b})
		}
	}
}

func (r *JSONDecoder) parseObject(out chan<- StreamItem) error {
	out <- &StartObject{}
	b, err := skipSpace(r.buf)
	if err != nil {
		return err
	}
	if b == '}' {
		out <- &EndObject{}
		return nil
	}
	for {
		if b != '"' {
			return fmt.Errorf("syntax error: expected '\"' for %q", b)
		}
		err := r.parseKey(out)
		if err != nil {
			return err
		}
		b, err = skipSpace(r.buf)
		if err != nil {
			return err
		}
		if b != ':' {
			return fmt.Errorf("syntax error: expected ':' got %q", b)
		}
		err = r.parseValue(out)
		if err != nil {
			return err
		}
		b, err = skipSpace(r.buf)
		if err != nil {
			return err
		}
		switch b {
		case '}':
			out <- &EndObject{}
			return nil
		case ',':
			b, err = skipSpace(r.buf)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("syntax error: expected '}' or ',' got %q", b)
		}
	}
}

func (r *JSONDecoder) parseTrue(out chan<- StreamItem) error {
	isTrue, err := check(r.buf, []byte("rue"))
	if err != nil {
		return err
	}
	if !isTrue {
		return errors.New("syntax error: expected true")
	}
	out <- trueInstance
	return nil
}

func (r *JSONDecoder) parseFalse(out chan<- StreamItem) error {
	isFalse, err := check(r.buf, []byte("alse"))
	if err != nil {
		return err
	}
	if !isFalse {
		return errors.New("syntax error")
	}
	out <- falseInstance
	return nil
}

func (r *JSONDecoder) parseNull(out chan<- StreamItem) error {
	isNull, err := check(r.buf, []byte("ull"))
	if err != nil {
		return err
	}
	if !isNull {
		return errors.New("syntax error")
	}
	out <- nullInstance
	return nil
}

func (r *JSONDecoder) parseNumber(b byte, out chan<- StreamItem) error {
	var err error
	var n int
	var numberBytes []byte

	// Sign part
	if b == '-' {
		numberBytes = append(numberBytes, b)
		b, err = r.buf.ReadByte()
	}
	if err != nil {
		return err
	}

	// Integer part
	if b == '0' {
		numberBytes = append(numberBytes, b)
		b, err = r.buf.ReadByte()
		if err != nil {
			return err
		}
	} else if b >= '1' && b <= '9' {
		b, _, err = readDigits(r.buf, b, &numberBytes)
		if err != nil {
			return err
		}
	} else {
		return errors.New("syntax error")
	}

	// Fraction part
	if b == '.' {
		numberBytes = append(numberBytes, b)
		b, err = r.buf.ReadByte()
		if err != nil {
			return err
		}
		b, n, err = readDigits(r.buf, b, &numberBytes)
		if err != nil {
			return err
		}
		if n == 0 {
			return errors.New("syntax error")
		}
	}

	// Exponent part
	if b == 'e' || b == 'E' {
		numberBytes = append(numberBytes, b)
		b, err = r.buf.ReadByte()
		if err != nil {
			return err
		}
		if b == '-' || b == '+' {
			numberBytes = append(numberBytes, b)
			b, err = r.buf.ReadByte()
			if err != nil {
				return err
			}
		}
		_, n, err = readDigits(r.buf, b, &numberBytes)
		if err != nil {
			return err
		}
		if n == 0 {
			return errors.New("syntax error")
		}
	}
	r.buf.UnreadByte()
	out <- NewScalar(Number, numberBytes)
	return nil
}

func parseString(buf *bufio.Reader) (*Scalar, error) {
	stringBytes := []byte{'"'}
	for {
		b, err := buf.ReadByte()
		if err != nil {
			return nil, err
		}
		switch b {
		case '\\':
			stringBytes = append(stringBytes, b)
			x, err := buf.ReadByte()
			if err != nil {
				return nil, err
			}
			stringBytes = append(stringBytes, x)
			switch x {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				continue
			case 'u':
				hex := make([]byte, 4)
				_, err := io.ReadFull(buf, hex)
				if err != nil {
					return nil, err
				}
				stringBytes = append(stringBytes, hex...)
				for _, d := range hex {
					if !(d >= '0' && d <= '9' || d >= 'a' && d <= 'f' || d >= 'A' && d <= 'F') {
						return nil, fmt.Errorf("syntax error: expected hex, got %q", d)
					}
				}
			}
		case '"':
			stringBytes = append(stringBytes, '"')
			return NewScalar(String, stringBytes), nil
		default:
			stringBytes = append(stringBytes, b)
		}
	}
}

func readDigits(reader *bufio.Reader, b byte, appendTo *[]byte) (byte, int, error) {
	var err error
	var n int
	for b >= '0' && b <= '9' {
		if appendTo != nil {
			*appendTo = append(*appendTo, b)
		}
		n++
		b, err = reader.ReadByte()
		if err != nil {
			return 0, 0, err
		}
	}
	return b, n, nil
}

func check(reader *bufio.Reader, expected []byte) (bool, error) {
	b := make([]byte, len(expected))
	_, err := io.ReadFull(reader, b)
	if err != nil {
		return false, err
	}
	eq := bytes.Equal(b, expected)
	return eq, nil
}

func skipSpace(reader *bufio.Reader) (byte, error) {
	for {
		b, error := reader.ReadByte()
		if error != nil {
			return b, error
		}
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		default:
			return b, nil
		}
	}
}

var (
	trueBytes  = []byte("true")
	falseBytes = []byte("false")
	nullBytes  = []byte("null")
)

var (
	trueInstance  = NewScalar(Boolean, trueBytes)
	falseInstance = NewScalar(Boolean, falseBytes)
	nullInstance  = NewScalar(Null, nullBytes)
)
