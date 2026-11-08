package jsonpath_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/arnodel/jsonstream/encoding/json"
	"github.com/arnodel/jsonstream/token"
)

// TestDescendantSegmentStreams checks that a descendant segment reads the
// values it doesn't select as they stream by.  $..email selects nothing from
// the root object, so diving into its "items" must not keep the items in
// memory.
func TestDescendantSegmentStreams(t *testing.T) {
	const itemCount = 10000
	var doc strings.Builder
	doc.WriteString(`{"meta": {"version": 1}, "items": [`)
	for i := range itemCount {
		if i > 0 {
			doc.WriteString(", ")
		}
		doc.WriteString(`{"id": 1, "email": "a@b.c", "address": {"city": "x"}}`)
	}
	doc.WriteString(`]}`)

	for _, query := range []string{`$..email`, `$..address.city`, `$..[?@.id == 2]`} {
		t.Run(query, func(t *testing.T) {
			acc := token.NewAccumulatorStream()
			if err := json.NewDecoder(strings.NewReader(doc.String())).Produce(acc); err != nil {
				t.Fatal(err)
			}
			runner, err := compileQueryString(query)
			if err != nil {
				t.Fatal(err)
			}
			var pools []*token.CursorPool
			defer token.ObservePools(func(p *token.CursorPool) { pools = append(pools, p) })()

			// Run the query in this goroutine, checking the pools whenever
			// it reads a token.
			maxBuffered := 0
			in := token.NewSliceReadStream(acc.GetTokens())
			next := nextFunc(func() token.Token {
				// A pool without cursors is finished with.
				pools = slices.DeleteFunc(pools, func(p *token.CursorPool) bool { return p.LiveCursors() == 0 })
				for _, p := range pools {
					maxBuffered = max(maxBuffered, p.Buffered())
				}
				return in.Next()
			})
			runner.Transform(next, token.NewAccumulatorStream())

			// An item is 12 tokens; the whole document is over 120,000.
			if maxBuffered > 1000 {
				t.Errorf("pool kept %d tokens", maxBuffered)
			}
		})
	}
}

type nextFunc func() token.Token

func (f nextFunc) Next() token.Token { return f() }
