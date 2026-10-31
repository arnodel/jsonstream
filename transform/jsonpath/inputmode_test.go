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
// of the scalar tokens it outputs, joined by spaces.
func collectScalars(t *testing.T, query string, in token.Stream, opts ...jsonpathtransformer.CompileQueryOption) string {
	t.Helper()
	runner, err := jsonpathtransformer.CompileQueryString(query, opts...)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	r := token.TransformStream(in, runner).Reader()
	for tok := r.Next(); tok != nil; tok = r.Next() {
		if s, ok := tok.(*token.Scalar); ok {
			got = append(got, string(s.Bytes))
		}
	}
	return strings.Join(got, " ")
}

func jsonInput(s string) token.Stream {
	return token.StartStream(json.NewDecoder(strings.NewReader(s)), nil)
}

func TestInputMode_MultipleValues(t *testing.T) {
	const input = `[1, 2, 3] [4, 5]`
	cases := []struct {
		name string
		opts []jsonpathtransformer.CompileQueryOption
		want string
	}{
		{"default is stream", nil, "1 4"},
		{"stream", []jsonpathtransformer.CompileQueryOption{jsonpathtransformer.WithInputMode(jsonpathtransformer.ValueStreamInput)}, "1 4"},
		{"single", []jsonpathtransformer.CompileQueryOption{jsonpathtransformer.WithInputMode(jsonpathtransformer.SingleValueInput)}, "1"},
		// Small values: the first ends within the budget so the input is a stream.
		{"auto", []jsonpathtransformer.CompileQueryOption{jsonpathtransformer.WithInputMode(jsonpathtransformer.AutoDetectInput)}, "1 4"},
		// A budget smaller than the first value: it is taken for a single value.
		{"auto, tiny budget", []jsonpathtransformer.CompileQueryOption{
			jsonpathtransformer.WithInputMode(jsonpathtransformer.AutoDetectInput),
			jsonpathtransformer.WithAutoDetectBudget(1),
		}, "1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := collectScalars(t, "$[:1]", jsonInput(input), c.opts...); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestInputMode_AutoDecidesOnce(t *testing.T) {
	// Once the input is found to be a stream, later values are read in full
	// even if they are bigger than the budget.
	big := "[" + strings.Repeat("0,", 50) + "0]"
	input := "[1, 2] " + big + " [3, 4]"
	got := collectScalars(t, "$[:1]", jsonInput(input),
		jsonpathtransformer.WithInputMode(jsonpathtransformer.AutoDetectInput),
		jsonpathtransformer.WithAutoDetectBudget(10),
	)
	if want := "1 0 3"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInputMode_ConsumedValueContinues(t *testing.T) {
	// When the query reads a value to its end, following values are still
	// processed even in single value mode: there is nothing to skip.
	got := collectScalars(t, "$..a", jsonInput(`{"a": 1} {"a": 2}`),
		jsonpathtransformer.WithInputMode(jsonpathtransformer.SingleValueInput))
	if got != "1 2" {
		t.Errorf("got %q, want %q", got, "1 2")
	}
}

func TestIsSingular(t *testing.T) {
	for q, want := range map[string]bool{"$": true, "$.a.b[0]": true, "$['a']": true, "$.a[*]": false, "$..a": false, "$[0, 1]": false, "$[?@.a]": false, "$[:1]": false} {
		r, err := jsonpathtransformer.CompileQueryString(q)
		if err != nil {
			t.Fatal(err)
		}
		if r.IsSingular() != want {
			t.Errorf("%s: IsSingular() = %v, want %v", q, r.IsSingular(), want)
		}
	}
}

// infiniteSource returns a stream fed forever by produce on its own goroutine.
// Tokens are sent in small batches so that the consumer gets them promptly.
func infiniteSource(produce func(token.WriteStream)) token.Stream {
	ch := make(chan []token.Token)
	go produce(token.NewBatchWriter(ch, 4))
	return ch
}

// collectOrTimeout is collectScalars with a guard against the query never
// returning, which is the failure mode these tests look for.
func collectOrTimeout(t *testing.T, query string, in token.Stream, opts ...jsonpathtransformer.CompileQueryOption) string {
	t.Helper()
	done := make(chan string, 1)
	go func() { done <- collectScalars(t, query, in, opts...) }()
	select {
	case got := <-done:
		return got
	case <-time.After(10 * time.Second):
		t.Fatalf("query %s did not stop reading an infinite input", query)
		return ""
	}
}

func TestInputMode_InfiniteInput(t *testing.T) {
	cases := []struct {
		name    string
		query   string
		produce func(token.WriteStream)
		want    string
	}{
		{
			name:  "slice of a root array that never ends",
			query: "$[:3]",
			produce: func(ch token.WriteStream) {
				ch.Put(&token.StartArray{})
				for n := 1; ; n++ {
					ch.Put(token.Int64Scalar(int64(n)))
				}
			},
			want: "1 2 3",
		},
		{
			name:  "name selector on an object that never ends",
			query: "$.name",
			produce: func(ch token.WriteStream) {
				ch.Put(&token.StartObject{})
				ch.Put(token.NewKey(token.String, []byte(`"name"`)))
				ch.Put(token.StringScalar("Alice"))
				for {
					ch.Put(token.NewKey(token.String, []byte(`"filler"`)))
					ch.Put(token.NullScalar)
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
			produce: func(ch token.WriteStream) {
				ch.Put(&token.StartObject{})
				ch.Put(token.NewKey(token.String, []byte(`"items"`)))
				ch.Put(&token.StartArray{})
				for n := 1; ; n++ {
					ch.Put(token.Int64Scalar(int64(n)))
				}
			},
			want: "1 2 3",
		},
		{
			name:  "index selector in a nested array that never ends",
			query: "$.items[1].id",
			produce: func(ch token.WriteStream) {
				ch.Put(&token.StartObject{})
				ch.Put(token.NewKey(token.String, []byte(`"items"`)))
				ch.Put(&token.StartArray{})
				for n := 0; ; n++ {
					ch.Put(&token.StartObject{})
					ch.Put(token.NewKey(token.String, []byte(`"id"`)))
					ch.Put(token.Int64Scalar(int64(n)))
					ch.Put(&token.EndObject{})
				}
			},
			want: "1",
		},
	}
	modes := []struct {
		name string
		mode jsonpathtransformer.InputMode
	}{{"single", jsonpathtransformer.SingleValueInput}, {"auto", jsonpathtransformer.AutoDetectInput}}
	for _, m := range modes {
		for _, c := range cases {
			t.Run(m.name+"/"+c.name, func(t *testing.T) {
				got := collectOrTimeout(t, c.query, infiniteSource(c.produce), jsonpathtransformer.WithInputMode(m.mode))
				if got != c.want {
					t.Errorf("got %q, want %q", got, c.want)
				}
			})
		}
	}
}
