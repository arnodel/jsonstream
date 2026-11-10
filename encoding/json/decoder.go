package json

import (
	"fmt"
	"io"

	"github.com/arnodel/jsonstream/internal/scanner"
	"github.com/arnodel/jsonstream/token"
)

// A Decoder reads JSON input and streams it into a JSON stream.
type Decoder struct {
	text  *textDecoder          // nil when created from a scanner
	scanr *scanner.Scanner      // nil when created with NewDecoder
	in    *token.FlushingReader // nil when created from a scanner
}

var _ token.StreamSource = &Decoder{}

// NewDecoder sets up a new Decoder instance to read from the given input.  It
// parses with encoding/json/jsontext, and on invalid input Produce returns a
// *SyntaxError.
func NewDecoder(in io.Reader) *Decoder {
	fr := &token.FlushingReader{Reader: in}
	return &Decoder{text: newTextDecoder(fr), in: fr}
}

// Produce reads a stream of JSON values and streams them, until it runs
// out of input or encounter invalid JSON, in which case it will return an
// error.
func (d *Decoder) Produce(out token.WriteStream) error {
	if d.in != nil {
		// Send the tokens produced so far before blocking on input.
		d.in.Flush = token.FlushFunc(out)
	}
	if d.text != nil {
		return d.text.produce(out)
	}
	for {
		_, err := d.scanr.SkipSpaceAndPeek()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		err = d.ParseValue(out)
		if err != nil {
			return err
		}
	}
}

// parseValue reads a single JSON value and streams it.  It can return a
// non-nil error if the input is invalid JSON.
func (d *Decoder) ParseValue(out token.WriteStream) error {
	b, err := d.scanr.SkipSpaceAndPeek()
	if err != nil {
		return err
	}
	switch b {
	case '"':
		s, err := ParseString(d.scanr)
		if err != nil {
			return err
		}
		out.Put(s)
		return nil
	case '[':
		return d.parseArray(out)
	case '{':
		return d.parseObject(out)
	case 't':
		err := checkBytes(d.scanr, trueBytes)
		if err != nil {
			return err
		}
		out.Put(trueInstance)
		return nil
	case 'f':
		err := checkBytes(d.scanr, falseBytes)
		if err != nil {
			return err
		}
		out.Put(falseInstance)
		return nil
	case 'n':
		err := checkBytes(d.scanr, nullBytes)
		if err != nil {
			return err
		}
		out.Put(nullInstance)
		return nil
	default:
		if b == '-' || b >= '0' && b <= '9' {
			n, err := ParseNumber(d.scanr)
			if err != nil {
				return err
			}
			out.Put(n)
			return nil
		}
		return UnexpectedByte(d.scanr, "unexpected")
	}
}

func (d *Decoder) parseArray(out token.WriteStream) error {
	var b byte
	var err error
	err = ExpectByte(d.scanr, '[')
	if err != nil {
		return err
	}
	out.Put(&token.StartArray{})
	b, err = d.scanr.SkipSpaceAndPeek()
	if err != nil {
		return err
	}
	if b == ']' {
		d.scanr.Read()
		out.Put(&token.EndArray{})
		return nil
	}
	for {
		err = d.ParseValue(out)
		if err != nil {
			return err
		}
		b, err = d.scanr.SkipSpaceAndPeek()
		if err != nil {
			return err
		}
		switch b {
		case ']':
			d.scanr.Read()
			out.Put(&token.EndArray{})
			return nil
		case ',':
			d.scanr.Read()
		default:
			return UnexpectedByte(d.scanr, "expected ']' or ',', got")
		}
	}
}

func (d *Decoder) parseObject(out token.WriteStream) error {
	var b byte
	err := ExpectByte(d.scanr, '{')
	if err != nil {
		return err
	}
	out.Put(&token.StartObject{})
	b, err = d.scanr.SkipSpaceAndPeek()
	if err != nil {
		return err
	}
	if b == '}' {
		d.scanr.Read()
		out.Put(&token.EndObject{})
		return nil
	}
	for {
		key, err := ParseString(d.scanr)
		if err != nil {
			return err
		}
		key.TypeAndFlags |= token.KeyMask
		out.Put(key)
		b, err = d.scanr.SkipSpaceAndPeek()
		if err != nil {
			return err
		}
		if b != ':' {
			return UnexpectedByte(d.scanr, "expected ':', got")
		}
		d.scanr.Read()
		err = d.ParseValue(out)
		if err != nil {
			return err
		}
		b, err = d.scanr.SkipSpaceAndPeek()
		if err != nil {
			return err
		}
		switch b {
		case '}':
			d.scanr.Read()
			out.Put(&token.EndObject{})
			return nil
		case ',':
			d.scanr.Read()
			b, err = d.scanr.SkipSpaceAndPeek()
			if err != nil {
				return err
			}
		default:
			return UnexpectedByte(d.scanr, "expected '}' or ',' got")
		}
	}
}

func ExpectByte(scanr *scanner.Scanner, xb byte) error {
	b, err := scanr.Read()
	if err != nil && err != io.EOF {
		return err
	}
	if err == io.EOF || b != xb {
		scanr.Back()
		return UnexpectedByte(scanr, "expected %q, got", xb)
	}
	return nil
}

func UnexpectedByte(scanr *scanner.Scanner, expected string, args ...interface{}) error {
	pos := scanr.CurrentPos()
	b, err := scanr.Read()
	if err != nil && err != io.EOF {
		return err
	}
	var got string
	switch {
	case err == io.EOF:
		got = "<EOF>"
	case b >= 0x80:
		// Not a character on its own: %q would show it as the code point.
		got = fmt.Sprintf(`'\x%02x'`, b)
	default:
		got = fmt.Sprintf("%q", b)
	}
	return fmt.Errorf("syntax error at L%d,C%d: %s: %s", pos.Line+1, pos.Col+1, fmt.Sprintf(expected, args...), got)
}

func ParseString(scanr *scanner.Scanner) (*token.Scalar, error) {
	scanr.StartToken()
	err := ExpectByte(scanr, '"')
	if err != nil {
		return nil, err
	}
	isAlnum := true
	isUnescaped := true
	firstChar := true
	for {
		b, err := scanr.Read()
		if err != nil {
			return nil, err
		}
		switch b {
		case '\\':
			isUnescaped = false
			isAlnum = false
			x, err := scanr.Read()
			if err != nil {
				return nil, err
			}
			switch x {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				continue
			case 'u':
				for i := 0; i < 4; i++ {
					b, err = scanr.Read()
					if err != nil {
						return nil, err
					}
					if !(b >= '0' && b <= '9' || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F') {
						scanr.Back()
						return nil, UnexpectedByte(scanr, "expected hex, got")
					}
				}
			default:
				scanr.Back()
				return nil, UnexpectedByte(scanr, "invalid escape character in string")
			}
		case '"':
			scalar := endScalar(scanr, token.String)
			// An empty string is not alphanumeric: it cannot be written as a
			// shorthand name in a path.
			if isAlnum && !firstChar {
				scalar.TypeAndFlags |= token.AlnumMask
			}
			if isUnescaped {
				scalar.TypeAndFlags |= token.UnescapedMask
			}
			return scalar, nil
		default:
			if scanner.IsCtrl(b) {
				scanr.Back()
				return nil, UnexpectedByte(scanr, "invalid control character in string")
			}
			if isAlnum {
				if firstChar {
					isAlnum = scanner.IsAlpha(b)
					firstChar = false
				} else {
					isAlnum = scanner.IsAlnum(b)
				}
			}
		}
	}
}

// ParseNumber parses a JSON number from the scanner. Exported for use by other format decoders.
func ParseNumber(scanr *scanner.Scanner) (*token.Scalar, error) {
	scanr.StartToken()
	var n int
	b, err := scanr.Read()

	// Sign part
	if b == '-' {
		b, err = scanr.Read()
	}
	if err != nil {
		return nil, err
	}

	// The input can end anywhere after the first digit, so from here on
	// io.EOF is not an error: b is then 0, which ends the number.

	// Integer part
	if b == '0' {
		b, err = scanr.Read()
		if err != nil && err != io.EOF {
			return nil, err
		}
	} else if b >= '1' && b <= '9' {
		b, _, err = ReadDigits(scanr)
		if err != nil {
			return nil, err
		}
	} else {
		scanr.Back()
		return nil, UnexpectedByte(scanr, "expected digit, got")
	}

	// Fraction part
	if b == '.' {
		b, n, err = ReadDigits(scanr)
		if err != nil {
			return nil, err
		}
		if n == 0 {
			scanr.Back()
			return nil, UnexpectedByte(scanr, "expected digit, got")
		}
	}

	// Exponent part
	if b == 'e' || b == 'E' {
		b, err = scanr.Peek()
		if err != nil && err != io.EOF {
			return nil, err
		}
		if b == '-' || b == '+' {
			scanr.Read()
		}
		_, n, err = ReadDigits(scanr)
		if err != nil {
			return nil, err
		}
		if n == 0 {
			scanr.Back()
			return nil, UnexpectedByte(scanr, "expected digit, got")
		}
	}
	scanr.Back()
	return endScalar(scanr, token.Number), nil
}

// endScalar ends the token being read by scanr and returns it as a scalar.
func endScalar(scanr *scanner.Scanner, tp token.ScalarType) *token.Scalar {
	b, borrowed := scanr.EndTokenBorrowed()
	if borrowed {
		return token.CopyScalar(tp, b)
	}
	return token.NewScalar(tp, b)
}

// ReadDigits reads digits and the byte after them, which it returns with the
// number of digits.  At the end of input that byte is 0.
func ReadDigits(scanr *scanner.Scanner) (byte, int, error) {
	var n int
	for {
		b, err := scanr.Read()
		if err == io.EOF {
			return 0, n, nil
		}
		if err != nil {
			return 0, n, err
		}
		if !scanner.IsDigit(b) {
			return b, n, nil
		}
		n++
	}
}

func checkBytes(scanr *scanner.Scanner, expected []byte) error {
	for _, xb := range expected {
		if err := ExpectByte(scanr, xb); err != nil {
			return err
		}
	}
	return nil
}

var (
	trueBytes  = []byte("true")
	falseBytes = []byte("false")
	nullBytes  = []byte("null")
)

var (
	trueInstance  = token.NewScalar(token.Boolean, trueBytes)
	falseInstance = token.NewScalar(token.Boolean, falseBytes)
	nullInstance  = token.NewScalar(token.Null, nullBytes)
)

// NewDecoderFromScanner creates a decoder using an existing scanner.
// This is useful for format decoders that need to parse JSON values inline.
// Unlike a decoder from NewDecoder, which reads with encoding/json/jsontext,
// it parses with the scanner, and accepts and rejects the same inputs (see
// FuzzDecoderMatchesScanner).
func NewDecoderFromScanner(scanr *scanner.Scanner) *Decoder {
	return &Decoder{scanr: scanr}
}
