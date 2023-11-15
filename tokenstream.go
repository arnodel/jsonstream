package jsonstream

type TokenReader interface {
	Read([]Token) (int, error)
}

type TokenWriter interface {
	Write([]Token)
}

type TokenReadStream interface {
	Next() Token
}

type TokenWriteStream interface {
	Put(Token)
}

type TokenReadBuf struct {
	reader TokenReader
	buf    []Token
	pos    int
}

type ChannelTokenReadStream <-chan Token

func (r ChannelTokenReadStream) Next() Token {
	return <-r
}

type RestartableTokenReadStream struct {
	stream   TokenReadStream
	consumed []Token
	index    int
}

func (r *RestartableTokenReadStream) Next() Token {
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

func (r *RestartableTokenReadStream) Restart() {
	r.index = 0
}
