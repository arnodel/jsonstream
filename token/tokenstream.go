package token

type Reader interface {
	Read([]Token) (int, error)
}

type Writer interface {
	Write([]Token)
}

type ReadStream interface {
	Next() Token
}

type WriteStream interface {
	Put(Token)
}

type ChannelReadStream <-chan Token

func (r ChannelReadStream) Next() Token {
	return <-r
}

type SliceReadStream struct {
	toks []Token
}

func NewSliceReadStream(toks []Token) *SliceReadStream {
	return &SliceReadStream{toks: toks}
}

func (r *SliceReadStream) Next() (tok Token) {
	if len(r.toks) > 0 {
		tok = r.toks[0]
		r.toks = r.toks[1:]
	}
	return
}
