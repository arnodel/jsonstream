package json

import (
	"fmt"
	"io"

	"github.com/arnodel/jsonstream/internal/scanner"
	"github.com/arnodel/jsonstream/token"
)

// A Decoder reads JSON input and streams it into a JSON stream.
type Decoder struct {
	scanr *scanner.Scanner
}

var _ token.StreamSource = &Decoder{}

// NewDecoder sets up a new Decoder instance to read from the given input.
func NewDecoder(in io.Reader) *Decoder {
	return &Decoder{scanr: scanner.NewScanner(in)}
}

// Produce reads a stream of JSON values and streams them, until it runs
// out of input or encounter invalid JSON, in which case it will return an
// error.
func (d *Decoder) Produce(out chan<- token.Token) error {
	for {
		b, err := d.scanr.SkipSpaceAndPeek()
		if err != nil || b == scanner.EOF {
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
func (d *Decoder) ParseValue(out chan<- token.Token) error {
	b, err := d.scanr.SkipSpaceAndPeek()
	if err != nil {
		return err
	}
	if b == scanner.EOF {
		return io.EOF
	}
	switch b {
	case '"':
		s, err := ParseString(d.scanr)
		if err != nil {
			return err
		}
		out <- s
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
		out <- trueInstance
		return nil
	case 'f':
		err := checkBytes(d.scanr, falseBytes)
		if err != nil {
			return err
		}
		out <- falseInstance
		return nil
	case 'n':
		err := checkBytes(d.scanr, nullBytes)
		if err != nil {
			return err
		}
		out <- nullInstance
		return nil
	default:
		if b == '-' || b >= '0' && b <= '9' {
			n, err := ParseNumber(d.scanr)
			if err != nil {
				return err
			}
			out <- n
			return nil
		}
		return UnexpectedByte(d.scanr, "unexpected")
	}
}

func (d *Decoder) parseArray(out chan<- token.Token) error {
	var b byte
	var err error
	err = ExpectByte(d.scanr, '[')
	if err != nil {
		return err
	}
	out <- &token.StartArray{}
	b, err = d.scanr.SkipSpaceAndPeek()
	if err != nil {
		return err
	}
	if b == ']' {
		d.scanr.Read()
		out <- &token.EndArray{}
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
			out <- &token.EndArray{}
			return nil
		case ',':
			d.scanr.Read()
		default:
			return UnexpectedByte(d.scanr, "expected ']' or ',', got")
		}
	}
}

func (d *Decoder) parseObject(out chan<- token.Token) error {
	var b byte
	err := ExpectByte(d.scanr, '{')
	if err != nil {
		return err
	}
	out <- &token.StartObject{}
	b, err = d.scanr.SkipSpaceAndPeek()
	if err != nil {
		return err
	}
	if b == '}' {
		d.scanr.Read()
		out <- &token.EndObject{}
		return nil
	}
	for {
		key, err := ParseString(d.scanr)
		if err != nil {
			return err
		}
		key.TypeAndFlags |= token.KeyMask
		out <- key
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
			out <- &token.EndObject{}
			return nil
		case ',':
			d.scanr.Read()
			_, err = d.scanr.SkipSpaceAndPeek()
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
	if err != nil {
		return err
	}
	if b != xb {
		scanr.Back()
		return UnexpectedByte(scanr, "expected %q, got", xb)
	}
	return nil
}

func UnexpectedByte(scanr *scanner.Scanner, expected string, args ...interface{}) error {
	pos := scanr.CurrentPos()
	b, err := scanr.Read()
	if err != nil {
		return err
	}
	if b == scanner.EOF {
		return fmt.Errorf("syntax error at L%d,C%d: %s: <EOF>", pos.Line+1, pos.Col+1, fmt.Sprintf(expected, args...))
	} else {
		return fmt.Errorf("syntax error at L%d,C%d: %s: %q", pos.Line+1, pos.Col+1, fmt.Sprintf(expected, args...), b)
	}
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
			}
		case '"':
			stringBytes := scanr.EndToken()
			scalar := token.NewScalar(token.String, stringBytes)
			if isAlnum {
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

	// Integer part
	if b == '0' {
		b, err = scanr.Read()
		if err != nil {
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
		if err != nil {
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
	return token.NewScalar(token.Number, scanr.EndToken()), nil
}

func ReadDigits(scanr *scanner.Scanner) (byte, int, error) {
	var n int
	for {
		b, err := scanr.Read()
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
func NewDecoderFromScanner(scanr *scanner.Scanner) *Decoder {
	return &Decoder{scanr: scanr}
}
