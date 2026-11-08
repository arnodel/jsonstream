package token

import (
	"bytes"
	"testing"
)

func TestCopyScalar(t *testing.T) {
	for _, n := range []int{0, 1, 8, 9, 16, 17, 32, 33, 64, 65, 200} {
		src := bytes.Repeat([]byte{'7'}, n)
		s := CopyScalar(Number, src)
		if s.Type() != Number {
			t.Errorf("len %d: got type %v", n, s.Type())
		}
		if !bytes.Equal(s.Bytes, src) {
			t.Errorf("len %d: got bytes %q", n, s.Bytes)
		}
		// The scalar must not share src.
		if n > 0 {
			src[0] = 'x'
			if s.Bytes[0] != '7' {
				t.Errorf("len %d: bytes share the source", n)
			}
		}
	}
}
