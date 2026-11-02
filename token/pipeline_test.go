package token_test

import (
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/arnodel/jsonstream/encoding/json"
	"github.com/arnodel/jsonstream/token"
)

// countTokens is a transformer passing tokens through unchanged.
type passThrough struct{}

func (passThrough) Transform(in token.ReadStream, out token.WriteStream) {
	for tok := in.Next(); tok != nil; tok = in.Next() {
		out.Put(tok)
	}
}

// TestPipelineLatency checks that batching does not hold back output: a value
// written to a pipe must come out of a decoder + transform pipeline before any
// further input is written, even though it is far smaller than a batch.
func TestPipelineLatency(t *testing.T) {
	pr, pw := io.Pipe()
	stream := token.StartStream(json.NewDecoder(pr), nil)
	stream = token.TransformStream(stream, passThrough{})
	r := stream.Reader()

	got := make(chan []token.Token, 1)
	go func() {
		var toks []token.Token
		for i := 0; i < 4; i++ {
			toks = append(toks, r.Next())
		}
		got <- toks
	}()

	if _, err := pw.Write([]byte(`{"a": 1}` + "\n")); err != nil {
		t.Fatal(err)
	}
	// The pipe stays open: the decoder is now blocked in Read.
	select {
	case toks := <-got:
		if len(toks) != 4 || toks[3] == nil {
			t.Fatalf("unexpected tokens %v", toks)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the first value did not come out of the pipeline while the input was blocked")
	}
	pw.Close()
	if tok := r.Next(); tok != nil {
		t.Errorf("expected end of stream, got %v", tok)
	}
}

func TestPipelineOrderAndCount(t *testing.T) {
	// More tokens than a batch, through two stages.
	const n = 1000
	src := sourceFunc(func(out token.WriteStream) error {
		for i := 0; i < n; i++ {
			out.Put(token.Int64Scalar(int64(i)))
		}
		return nil
	})
	stream := token.TransformStream(token.StartStream(src, nil), passThrough{})
	r := stream.Reader()
	for i := 0; i < n; i++ {
		tok := r.Next()
		s, ok := tok.(*token.Scalar)
		if !ok || string(s.Bytes) != token.Int64Scalar(int64(i)).String()[7:len(token.Int64Scalar(int64(i)).String())-1] {
			t.Fatalf("token %d: got %v", i, tok)
		}
	}
	if r.Next() != nil {
		t.Error("expected end of stream")
	}
}

type sourceFunc func(out token.WriteStream) error

func (f sourceFunc) Produce(out token.WriteStream) error { return f(out) }

// truncating copies its input and then reports that it ended inside a value,
// as a stage reading a truncated stream does.
type truncating struct{}

func (truncating) Transform(in token.ReadStream, out token.WriteStream) {
	for tok := in.Next(); tok != nil; tok = in.Next() {
		out.Put(tok)
	}
	panic(fmt.Errorf("%w: test", token.ErrTruncated))
}

func TestTransformStreamTruncated(t *testing.T) {
	// The source fails after starting an array, the stage downstream finds
	// the array cut short; what it produced must still come out, and the
	// stream must end rather than the program crash.
	src := sourceFunc(func(out token.WriteStream) error {
		out.Put(&token.StartArray{})
		out.Put(token.Int64Scalar(1))
		return errors.New("syntax error")
	})
	var sourceErr error
	stream := token.TransformStream(token.StartStream(src, func(err error) { sourceErr = err }), truncating{})
	r := stream.Reader()
	if _, ok := r.Next().(*token.StartArray); !ok {
		t.Fatal("expected StartArray")
	}
	if _, ok := r.Next().(*token.Scalar); !ok {
		t.Fatal("expected Scalar")
	}
	if tok := r.Next(); tok != nil {
		t.Fatalf("expected end of stream, got %v", tok)
	}
	if sourceErr == nil {
		t.Error("source error not reported")
	}
}

func TestCatchTruncated(t *testing.T) {
	err := func() (err error) {
		defer token.CatchTruncated(&err)
		panic(fmt.Errorf("%w: test", token.ErrTruncated))
	}()
	if !errors.Is(err, token.ErrTruncated) {
		t.Errorf("got %v, want ErrTruncated", err)
	}
	defer func() {
		if r := recover(); r != "other" {
			t.Errorf("other panics must carry on, got %v", r)
		}
	}()
	func() {
		var err error
		defer token.CatchTruncated(&err)
		panic("other")
	}()
}
