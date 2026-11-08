package jpv

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/arnodel/jsonstream/encoding/json"
	"github.com/arnodel/jsonstream/token"
)

func BenchmarkEncoder(b *testing.B) {
	var doc strings.Builder
	doc.WriteString(`{"meta": {"version": 1}, "items": [`)
	for i := range 10000 {
		if i > 0 {
			doc.WriteString(",")
		}
		fmt.Fprintf(&doc, `{"active":true,"address":{"city":"Boston","zip":"02101"},"email":"user%d@example.com","id":%d,"name":"user%d","price":%d.5,"tags":%s}`,
			i, i, i, i%200, []string{`[]`, `["a"]`, `["a","b"]`}[i%3])
	}
	doc.WriteString(`]}`)
	acc := token.NewAccumulatorStream()
	if err := json.NewDecoder(strings.NewReader(doc.String())).Produce(acc); err != nil {
		b.Fatal(err)
	}
	toks := acc.GetTokens()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := NewEncoder(io.Discard).Consume(token.NewSliceReadStream(toks)); err != nil {
			b.Fatal(err)
		}
	}
}
