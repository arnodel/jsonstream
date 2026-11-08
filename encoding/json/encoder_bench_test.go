package json

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/arnodel/jsonstream/token"
)

// benchItems returns the tokens of a document like the one 'bench gen' makes,
// with n items.
func benchItems(b *testing.B, n int) []token.Token {
	var doc strings.Builder
	doc.WriteString(`{"meta": {"version": 1}, "items": [`)
	for i := range n {
		if i > 0 {
			doc.WriteString(",")
		}
		fmt.Fprintf(&doc, `{"active":true,"address":{"city":"Boston","zip":"02101"},"email":"user%d@example.com","id":%d,"name":"user%d","price":%d.5,"tags":%s}`,
			i, i, i, i%200, []string{`[]`, `["a"]`, `["a","b"]`}[i%3])
	}
	doc.WriteString(`]}`)
	acc := token.NewAccumulatorStream()
	if err := NewDecoder(strings.NewReader(doc.String())).Produce(acc); err != nil {
		b.Fatal(err)
	}
	return acc.GetTokens()
}

func BenchmarkEncoder(b *testing.B) {
	toks := benchItems(b, 10000)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := NewEncoder(io.Discard).Consume(token.NewSliceReadStream(toks)); err != nil {
			b.Fatal(err)
		}
	}
}
