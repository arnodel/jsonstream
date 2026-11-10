package jpv

import (
	"errors"
	"io"

	"github.com/arnodel/jsonstream/encoding/json"
	"github.com/arnodel/jsonstream/internal/scanner"
	"github.com/arnodel/jsonstream/token"
)

// scannerDecoder is the JPV decoder as it was before it used jsontext, kept
// to check that the two decode the same inputs the same way (see
// FuzzJPVMatchesScanner).
type scannerDecoder struct {
	scanr *scanner.Scanner
	paths pathTracker
}

func newScannerDecoder(in io.Reader) *scannerDecoder {
	return &scannerDecoder{scanr: scanner.NewScanner(in)}
}

func (d *scannerDecoder) Produce(out token.WriteStream) error {
	defer d.paths.close(out)
	for {
		_, err := d.scanr.SkipSpaceAndPeek()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		err = d.parseLine(out)
		if err != nil {
			return err
		}
	}
}

func (d *scannerDecoder) parseLine(out token.WriteStream) error {
	err := json.ExpectByte(d.scanr, '$')
	if err != nil {
		return err
	}
	linePath, err := parseScannerPath(d.scanr)
	if err != nil {
		return err
	}
	// Peek rather than read and push back: parsePath may already have pushed
	// this byte back, and the scanner cannot go back twice.
	b, err := d.scanr.SkipSpaceAndPeek()
	if err != nil {
		return err
	}
	if b != '=' {
		return json.UnexpectedByte(d.scanr, "expected '=', got")
	}
	d.scanr.Read()
	err = d.paths.update(linePath, out)
	if err != nil {
		return err
	}
	jsonDecoder := json.NewDecoderFromScanner(d.scanr)
	return jsonDecoder.ParseValue(out)
}

func parseScannerPath(scanr *scanner.Scanner) ([]*token.Scalar, error) {
	var path []*token.Scalar
	for {
		b, err := scanr.Read()
		if err != nil {
			// That's ok because paths are followed by a value
			return nil, err
		}
		switch {
		case b == '[':
			b, err = scanr.Peek()
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
				b, err = scanr.Read()
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
				// ReadDigits has consumed the byte after the digits: exclude it
				// from the token, then read it again.
				scanr.Back()
				path = append(path, token.NewKey(token.Number, scanr.EndToken()))
				b, err = scanr.Read()
				if err != nil {
					return nil, err
				}
			}
			if b != ']' {
				return nil, errors.New("syntax error: expected ']'")
			}
		case b == '.':
			scanr.StartToken()
			b, err = scanr.Read()
			if err != nil {
				return nil, err
			}
			if !scanner.IsAlpha(b) {
				scanr.Back()
				return nil, json.UnexpectedByte(scanr, "expected a-z/A-Z/_, got")
			}
			for {
				b, err = scanr.Read()
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
