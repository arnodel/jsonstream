package token

import "testing"

func TestBatchWriterReader(t *testing.T) {
	ch := make(chan []Token, 10)
	w := NewBatchWriter(ch, 3)
	toks := make([]Token, 7)
	for i := range toks {
		toks[i] = Int64Scalar(int64(i))
		w.Put(toks[i])
	}
	// 7 tokens with batches of 3: two full batches sent, one token pending.
	if len(ch) != 2 {
		t.Fatalf("expected 2 batches sent, got %d", len(ch))
	}
	w.Flush()
	if len(ch) != 3 {
		t.Fatalf("expected 3 batches after Flush, got %d", len(ch))
	}
	w.Flush() // nothing pending: no empty batch
	if len(ch) != 3 {
		t.Fatalf("Flush with nothing pending sent a batch")
	}
	close(ch)

	r := NewBatchReader(ch)
	for i, want := range toks {
		if got := r.Next(); got != want {
			t.Errorf("token %d: got %v, want %v", i, got, want)
		}
	}
	for i := 0; i < 2; i++ {
		if got := r.Next(); got != nil {
			t.Errorf("expected nil after end of stream, got %v", got)
		}
	}
}

func TestBatchReaderBeforeBlock(t *testing.T) {
	ch := make(chan []Token, 1)
	r := NewBatchReader(ch)
	calls := 0
	r.BeforeBlock = func() {
		calls++
		// Unblock the reader: this is what a flush does for the stage
		// downstream, which in turn lets upstream make progress.
		ch <- []Token{NullScalar}
	}
	// A batch is already available: BeforeBlock must not be called.
	ch <- []Token{TrueScalar}
	if got := r.Next(); got != TrueScalar || calls != 0 {
		t.Fatalf("got %v, calls=%d; want TrueScalar, 0", got, calls)
	}
	// Nothing available: BeforeBlock is called before blocking.
	if got := r.Next(); got != NullScalar || calls != 1 {
		t.Fatalf("got %v, calls=%d; want NullScalar, 1", got, calls)
	}
}

func TestFlushFunc(t *testing.T) {
	if FlushFunc(NewAccumulatorStream()) != nil {
		t.Error("AccumulatorStream is not a Flusher")
	}
	ch := make(chan []Token, 1)
	w := NewBatchWriter(ch, 10)
	w.Put(NullScalar)
	flush := FlushFunc(w)
	if flush == nil {
		t.Fatal("BatchWriter should be a Flusher")
	}
	flush()
	if len(ch) != 1 {
		t.Error("flush did not send the pending batch")
	}
}
