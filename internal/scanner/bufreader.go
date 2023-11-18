package scanner

import "sync"

type Pipe[T any] struct {
	buf      []T
	readPos  int
	writePos int

	readLock  sync.Mutex
	writeLock sync.Mutex
}

func (p *Pipe[T]) Write(data []T) {

}
