package jpv

import (
	"errors"
	"io"

	"github.com/arnodel/jsonstream/encoding/json"
	"github.com/arnodel/jsonstream/internal/scanner"
	"github.com/arnodel/jsonstream/token"
)

// Decoder reads input in JPV format and streams it into a JSON stream.
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
type Decoder struct {
	scanr    *scanner.Scanner
	lastPath []*token.Scalar
}

var _ token.StreamSource = &Decoder{}

// NewDecoder sets up a new Decoder instance to read from the given
// input.
func NewDecoder(in io.Reader) *Decoder {
	return &Decoder{scanr: scanner.NewScanner(in)}
}

// Produce reads a stream of JPV values and streams them, until it runs out of
// input or encounters invalid JPV, in which case it will return an error.
func (d *Decoder) Produce(out chan<- token.Token) error {
	defer func() {
		unwindPath(d.lastPath, false, out)
	}()
	for {
		b, err := d.scanr.SkipSpaceAndPeek()
		if err != nil || b == scanner.EOF {
			return err
		}
		err = d.parseLine(out)
		if err != nil {
			return err
		}
	}
}

func (d *Decoder) parseLine(out chan<- token.Token) error {
	err := json.ExpectByte(d.scanr, '$')
	if err != nil {
		return err
	}
	linePath, err := parsePath(d.scanr)
	if err != nil {
		return err
	}
	b, err := checkEOF(d.scanr.SkipSpaceAndRead())
	if err != nil {
		return err
	}
	if b != '=' {
		d.scanr.Back()
		return json.UnexpectedByte(d.scanr, "expected '=', got")
	}
	err = d.updatePath(linePath, out)
	if err != nil {
		return err
	}
	// TODO: tidy this up
	jsonDecoder := json.NewDecoderFromScanner(d.scanr)
	return jsonDecoder.ParseValue(out)
}

func (d *Decoder) updatePath(newPath []*token.Scalar, out chan<- token.Token) error {
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
	unwindPath(d.lastPath[divergenceIndex:], true, out)

	// Open object and arrays the new object is in
	followPath(newPath[divergenceIndex:], true, out)
	d.lastPath = newPath
	return nil
}

func unwindPath(path []*token.Scalar, inCollection bool, out chan<- token.Token) {
	for i := len(path) - 1; i >= 0; i-- {
		if i > 0 || !inCollection {
			switch path[i].Type() {
			case token.String:
				out <- &token.EndObject{}
			case token.Number:
				out <- &token.EndArray{}
			default:
				panic("invalid key type (must be string or number)")
			}
		}
	}
}

func followPath(path []*token.Scalar, inCollection bool, out chan<- token.Token) {
	for _, key := range path {
		switch key.Type() {
		case token.String:
			if !inCollection {
				out <- &token.StartObject{}
			}
			out <- key
		case token.Number:
			if !inCollection {
				out <- &token.StartArray{}
			}
		default:
			panic("invalid key type (must be string or number)")
		}
		inCollection = false
	}
}

// checkEOF checks if there's an error or if the byte is scanner.EOF.
// If err is not nil, it returns (b, err).
// If b is scanner.EOF, it returns (0, io.EOF).
// Otherwise it returns (b, nil).
// This helper reduces the repetitive pattern of checking for scanner.EOF
// throughout the decoder by combining error checking and EOF detection.
func checkEOF(b byte, err error) (byte, error) {
	if err != nil {
		return b, err
	}
	if b == scanner.EOF {
		return 0, io.EOF
	}
	return b, nil
}

func parsePath(scanr *scanner.Scanner) ([]*token.Scalar, error) {
	var path []*token.Scalar
	for {
		b, err := checkEOF(scanr.Read())
		if err != nil {
			// That's ok because paths are followed by a value
			return nil, err
		}
		switch {
		case b == '[':
			b, err = checkEOF(scanr.Peek())
			if err != nil {
				return nil, err
			}
			if b == '"' {
				s, err := json.ParseString(scanr)
				if err != nil {
					return nil, err
				}
				s.TypeAndFlags |= token.KeyMask
				path = append(path, s)
				b, err = checkEOF(scanr.Read())
				if err != nil {
					return nil, err
				}
			} else {
				var n int
				scanr.StartToken()
				b, n, err = json.ReadDigits(scanr)
				if err != nil {
					return nil, err
				}
				if n == 0 {
					scanr.Back()
					return nil, json.UnexpectedByte(scanr, "expected digit, got")
				}
				path = append(path, token.NewKey(token.Number, scanr.EndToken()))
			}
			if b != ']' {
				return nil, errors.New("syntax error: expected ']'")
			}
		case b == '.':
			scanr.StartToken()
			b, err = checkEOF(scanr.Read())
			if err != nil {
				return nil, err
			}
			if !scanner.IsAlpha(b) {
				scanr.Back()
				return nil, json.UnexpectedByte(scanr, "expected a-z/A-Z/_, got")
			}
			for {
				b, err = checkEOF(scanr.Read())
				if err != nil {
					return nil, err
				}
				if !scanner.IsAlnum(b) {
					scanr.Back()
					keyBytes := scanr.EndToken()
					key := token.NewScalar(token.String, append(append([]byte{'"'}, keyBytes...), '"'))
					key.TypeAndFlags |= token.AlnumMask | token.KeyMask
					path = append(path, key)
					break
				}
			}
		default:
			scanr.Back()
			return path, nil
		}
	}
}
