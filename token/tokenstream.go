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

var _ ReadStream = make(ChannelReadStream)

func (r ChannelReadStream) Next() Token {
	return <-r
}

type SliceReadStream struct {
	toks []Token
}

var _ ReadStream = &SliceReadStream{}

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

type ChannelWriteStream chan<- Token

var _ WriteStream = make(ChannelWriteStream)

func (w ChannelWriteStream) Put(tok Token) {
	w <- tok
}

type AccumulatorStream struct {
	toks []Token
}

var _ WriteStream = &AccumulatorStream{}

func NewAccumulatorStream() *AccumulatorStream {
	return &AccumulatorStream{}
}

func (w *AccumulatorStream) Put(tok Token) {
	w.toks = append(w.toks, tok)
}

func (w *AccumulatorStream) GetTokens() []Token {
	return w.toks
}
