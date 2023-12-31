package jsonstream

import "github.com/arnodel/jsonstream/token"

type Colorizer struct {
	KeyColorCode     []byte
	ScalarColorCodes [4][]byte
	ResetCode        []byte
}

func (c *Colorizer) ScalarColorCode(scalar *token.Scalar) []byte {
	if scalar.IsKey() {
		return c.KeyColorCode
	}
	return c.ScalarColorCodes[scalar.Type()]
}

func (c *Colorizer) PrintScalar(p Printer, scalar *token.Scalar) {
	if c != nil {
		p.PrintBytes(c.ScalarColorCode(scalar))
	}
	p.PrintBytes(scalar.Bytes)
	if c != nil {
		p.PrintBytes(c.ResetCode)
	}
}

func (c *Colorizer) PrintSuccintScalar(p Printer, scalar *token.Scalar) {
	if c != nil {
		p.PrintBytes(c.ScalarColorCode(scalar))
	}
	if scalar.IsAlnum() {
		p.PrintBytes(scalar.Bytes[1 : len(scalar.Bytes)-1])
	} else {
		p.PrintBytes(scalar.Bytes)
	}
	if c != nil {
		p.PrintBytes(c.ResetCode)
	}
}
