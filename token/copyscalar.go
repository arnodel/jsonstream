package token

// CopyScalar returns a new scalar of type tp holding a copy of b.  Up to 64
// bytes, the scalar and its bytes are a single allocation, which is about
// twice as fast to decode as two; nothing else shares it, so keeping the
// scalar alive keeps nothing else alive.
func CopyScalar(tp ScalarType, b []byte) *Scalar {
	switch n := len(b); {
	case n <= 8:
		x := &struct {
			s Scalar
			b [8]byte
		}{}
		return x.s.init(tp, x.b[:n:n], b)
	case n <= 16:
		x := &struct {
			s Scalar
			b [16]byte
		}{}
		return x.s.init(tp, x.b[:n:n], b)
	case n <= 32:
		x := &struct {
			s Scalar
			b [32]byte
		}{}
		return x.s.init(tp, x.b[:n:n], b)
	case n <= 64:
		x := &struct {
			s Scalar
			b [64]byte
		}{}
		return x.s.init(tp, x.b[:n:n], b)
	default:
		return NewScalar(tp, append([]byte(nil), b...))
	}
}

func (s *Scalar) init(tp ScalarType, dst, src []byte) *Scalar {
	copy(dst, src)
	s.Bytes = dst
	s.TypeAndFlags = uint8(tp)
	return s
}
