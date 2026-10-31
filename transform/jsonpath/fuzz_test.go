package jsonpath_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	jpjson "github.com/arnodel/jsonstream/encoding/json"
	"github.com/arnodel/jsonstream/internal/format"
	"github.com/arnodel/jsonstream/iterator"
	"github.com/arnodel/jsonstream/token"
	jsonpathtransformer "github.com/arnodel/jsonstream/transform/jsonpath"
)

// runQueryCollect runs the query over the document and returns each result
// value as compact JSON, in output order.
func runQueryCollect(t *testing.T, runner jsonpathtransformer.MainQueryRunner, doc string) []string {
	t.Helper()
	stream := token.StartStream(jpjson.NewDecoder(strings.NewReader(doc)), nil)
	stream = token.TransformStream(stream, runner)
	iter := iterator.New(stream.Reader())
	var results []string
	for iter.Advance() {
		var buf bytes.Buffer
		enc := &jpjson.Encoder{Printer: &format.DefaultPrinter{Writer: &buf, IndentSize: -1}}
		acc := token.NewAccumulatorStream()
		iter.CurrentValue().Copy(acc)
		if err := enc.Consume(token.NewSliceReadStream(acc.GetTokens())); err != nil {
			t.Fatal(err)
		}
		results = append(results, strings.TrimSpace(buf.String()))
	}
	return results
}

// ctsSeeds returns the (selector, document) pairs of the compliance test
// suite, as seeds for the fuzzer and as the leak test's corpus.
func ctsSeeds(t testing.TB) [][2]string {
	data, err := os.ReadFile("cts.json")
	if err != nil {
		t.Fatal(err)
	}
	var cts struct {
		Tests []struct {
			Selector        string          `json:"selector"`
			Document        json.RawMessage `json:"document"`
			InvalidSelector bool            `json:"invalid_selector"`
		} `json:"tests"`
	}
	if err := json.Unmarshal(data, &cts); err != nil {
		t.Fatal(err)
	}
	var seeds [][2]string
	for _, test := range cts.Tests {
		if test.InvalidSelector || test.Document == nil {
			continue
		}
		seeds = append(seeds, [2]string{test.Selector, string(test.Document)})
	}
	return seeds
}

// FuzzQuery runs a query over a document in both modes and checks that
// neither panics, that they agree on the set of results (they may differ in
// order), and that no cursor is left pinning a pool's window afterwards.  Its
// seed corpus is the whole compliance suite, so 'go test' runs the leak check
// over every compliance query.
func FuzzQuery(f *testing.F) {
	for _, seed := range ctsSeeds(f) {
		f.Add(seed[0], seed[1])
	}
	f.Fuzz(func(t *testing.T, query, doc string) {
		relaxed, err := jsonpathtransformer.CompileQueryString(query)
		if err != nil {
			return
		}
		strict, err := jsonpathtransformer.CompileQueryString(query, jsonpathtransformer.WithStrictMode(true))
		if err != nil {
			if errors.Is(err, jsonpathtransformer.ErrUnimplementedFeature) {
				return
			}
			t.Fatalf("query compiles in default mode but not in strict mode: %v", err)
		}
		// Only run well-formed single documents: the decoder is fuzzed
		// separately, and the result-set comparison assumes one document.
		acc := token.NewAccumulatorStream()
		if err := jpjson.NewDecoder(strings.NewReader(doc)).Produce(acc); err != nil {
			return
		}
		if iter := iterator.New(token.NewSliceReadStream(acc.GetTokens())); !iter.Advance() || iter.Advance() {
			return
		}

		var pools []*token.CursorPool
		restore := token.ObservePools(func(p *token.CursorPool) { pools = append(pools, p) })
		defer restore()

		got := runQueryCollect(t, relaxed, doc)
		gotStrict := runQueryCollect(t, strict, doc)

		for _, p := range pools {
			if n := p.LiveCursors(); n != 0 {
				t.Fatalf("%d cursor(s) left attached after running %s on %s: the window is pinned", n, query, doc)
			}
		}
		a, b := slices.Clone(got), slices.Clone(gotStrict)
		slices.Sort(a)
		slices.Sort(b)
		if !slices.Equal(a, b) {
			t.Fatalf("default and strict modes disagree on %s over %s\ndefault: %v\nstrict:  %v", query, doc, got, gotStrict)
		}
	})
}
