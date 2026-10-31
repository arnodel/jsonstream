package jsonpath_test

import (
	"strings"
	"testing"
	"time"

	"github.com/arnodel/jsonstream/encoding/json"
	"github.com/arnodel/jsonstream/token"
	jsonpathtransformer "github.com/arnodel/jsonstream/transform/jsonpath"
)

// collectScalars runs the query over the input and returns the literal bytes
// of the scalar tokens it outputs.
func collectScalars(t *testing.T, query string, in <-chan token.Token, opts ...jsonpathtransformer.CompileQueryOption) []string {
	t.Helper()
	runner, err := jsonpathtransformer.CompileQueryString(query, opts...)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for tok := range token.TransformStream(in, runner) {
		if s, ok := tok.(*token.Scalar); ok {
			got = append(got, string(s.Bytes))
		}
	}
	return got
}

func TestSingleValue_MultipleValues(t *testing.T) {
	const input = `[1, 2, 3] [4, 5]`
	cases := []struct {
		name   string
		single bool
		want   string
	}{
		// Without the option, the query applies to every value in the stream.
		{"stream", false, "1 4"},
		// With it, the query stops after the first value's results.
		{"single", true, "1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := token.StartStream(json.NewDecoder(strings.NewReader(input)), nil)
			got := collectScalars(t, "$[:1]", in, jsonpathtransformer.WithSingleValue(c.single))
			if s := strings.Join(got, " "); s != c.want {
				t.Errorf("got %q, want %q", s, c.want)
			}
		})
	}
}

func TestSingleValue_ConsumedValueContinues(t *testing.T) {
	// When the query reads a value to its end, following values are still
	// processed even in single value mode: there is nothing to skip.
	in := token.StartStream(json.NewDecoder(strings.NewReader(`{"a": 1} {"a": 2}`)), nil)
	got := collectScalars(t, "$..a", in, jsonpathtransformer.WithSingleValue(true))
	if s := strings.Join(got, " "); s != "1 2" {
		t.Errorf("got %q, want %q", s, "1 2")
	}
}

// infiniteSource returns a channel fed forever by produce on its own goroutine.
func infiniteSource(produce func(chan<- token.Token)) <-chan token.Token {
	ch := make(chan token.Token)
	go produce(ch)
	return ch
}

// collectScalarsOrTimeout is collectScalars with a guard against the query
// never returning, which is the failure mode these tests look for.
func collectScalarsOrTimeout(t *testing.T, query string, in <-chan token.Token) []string {
	t.Helper()
	done := make(chan []string, 1)
	go func() { done <- collectScalars(t, query, in, jsonpathtransformer.WithSingleValue(true)) }()
	select {
	case got := <-done:
		return got
	case <-time.After(10 * time.Second):
		t.Fatalf("query %s did not stop reading an infinite input", query)
		return nil
	}
}

func TestSingleValue_InfiniteInput(t *testing.T) {
	cases := []struct {
		name    string
		query   string
		produce func(chan<- token.Token)
		want    string
	}{
		{
			name:  "slice of a root array that never ends",
			query: "$[:3]",
			produce: func(ch chan<- token.Token) {
				ch <- &token.StartArray{}
				for n := 1; ; n++ {
					ch <- token.Int64Scalar(int64(n))
				}
			},
			want: "1 2 3",
		},
		{
			name:  "name selector on an object that never ends",
			query: "$.name",
			produce: func(ch chan<- token.Token) {
				ch <- &token.StartObject{}
				ch <- token.NewKey(token.String, []byte(`"name"`))
				ch <- token.StringScalar("Alice")
				for {
					ch <- token.NewKey(token.String, []byte(`"filler"`))
					ch <- token.NullScalar
				}
			},
			want: `"Alice"`,
		},
		{
			// The typical shape: a huge array under a key.  The name selector
			// is done as soon as the key is found, and the slice is done after
			// 3 items; neither must cause the rest of the array to be read.
			name:  "slice of a nested array that never ends",
			query: "$.items[:3]",
			produce: func(ch chan<- token.Token) {
				ch <- &token.StartObject{}
				ch <- token.NewKey(token.String, []byte(`"items"`))
				ch <- &token.StartArray{}
				for n := 1; ; n++ {
					ch <- token.Int64Scalar(int64(n))
				}
			},
			want: "1 2 3",
		},
		{
			name:  "index selector in a nested array that never ends",
			query: "$.items[1].id",
			produce: func(ch chan<- token.Token) {
				ch <- &token.StartObject{}
				ch <- token.NewKey(token.String, []byte(`"items"`))
				ch <- &token.StartArray{}
				for n := 0; ; n++ {
					ch <- &token.StartObject{}
					ch <- token.NewKey(token.String, []byte(`"id"`))
					ch <- token.Int64Scalar(int64(n))
					ch <- &token.EndObject{}
				}
			},
			want: "1",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := collectScalarsOrTimeout(t, c.query, infiniteSource(c.produce))
			if s := strings.Join(got, " "); s != c.want {
				t.Errorf("got %q, want %q", s, c.want)
			}
		})
	}
}
