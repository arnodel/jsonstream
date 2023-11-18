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

type ReadBuf struct {
	reader Reader
	buf    []Token
	pos    int
}

type ChannelReadStream <-chan Token

func (r ChannelReadStream) Next() Token {
	return <-r
}

type RestartableReadStream struct {
	stream   ReadStream
	consumed []Token
	index    int
}

func NewRestartableReadStream(stream ReadStream) *RestartableReadStream {
	return &RestartableReadStream{
		stream: stream,
	}
}

func (r *RestartableReadStream) Next() Token {
	if r.index < len(r.consumed) {
		next := r.consumed[r.index]
		r.index++
		return next
	}
	next := r.stream.Next()
	if next != nil {
		r.consumed = append(r.consumed, next)
		r.index++
	}
	return next
}

func (r *RestartableReadStream) Restart() {
	r.index = 0
}
