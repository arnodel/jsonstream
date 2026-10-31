package token

import (
	"testing"
	"time"
)

func TestTickingReadStream(t *testing.T) {
	ch := make(chan Token)
	ticks := 0
	stream := NewTickingReadStream(ch, 10*time.Millisecond, func() { ticks++ })
	defer stream.Stop()

	toks := []Token{&StartArray{}, NewScalar(Number, []byte("1")), &EndArray{}}
	go func() {
		ch <- toks[0]
		time.Sleep(100 * time.Millisecond) // stall: ticks should fire
		ch <- toks[1]
		ch <- toks[2]
		close(ch)
	}()

	for i, want := range toks {
		got := stream.Next()
		if got != want {
			t.Errorf("token %d: got %v, want %v", i, got, want)
		}
	}
	if got := stream.Next(); got != nil {
		t.Errorf("expected nil after channel closed, got %v", got)
	}
	if ticks == 0 {
		t.Error("expected the callback to be called during the stall")
	}
}

func TestTickingReadStreamNoStall(t *testing.T) {
	ch := make(chan Token, 3)
	ch <- &StartArray{}
	ch <- &EndArray{}
	close(ch)
	stream := NewTickingReadStream(ch, time.Hour, func() { t.Error("unexpected tick") })
	defer stream.Stop()
	n := 0
	for stream.Next() != nil {
		n++
	}
	if n != 2 {
		t.Errorf("got %d tokens, want 2", n)
	}
}
