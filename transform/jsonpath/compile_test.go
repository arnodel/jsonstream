package jsonpath_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/arnodel/jsonstream/encoding/json"
	"github.com/arnodel/jsonstream/token"
	jsonpathtransformer "github.com/arnodel/jsonstream/transform/jsonpath"
)

func TestCompileQueryString(t *testing.T) {
	if _, err := jsonpathtransformer.CompileQueryString("$.a[*].b"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	for _, bad := range []string{"", "@.x", "$[", "$[?unknown(@)]"} {
		if _, err := jsonpathtransformer.CompileQueryString(bad); err == nil {
			t.Errorf("CompileQueryString(%q): expected an error", bad)
		}
	}
}

// ExampleCompileQueryString shows how to run a JSONPath query over a stream of
// JSON from Go code.
func ExampleCompileQueryString() {
	runner, err := jsonpathtransformer.CompileQueryString("$.users[?@.age > 30].name")
	if err != nil {
		panic(err)
	}
	input := strings.NewReader(`{"users": [{"name": "Alice", "age": 35}, {"name": "Bob", "age": 25}]}`)
	stream := token.StartStream(json.NewDecoder(input), nil)
	stream = token.TransformStream(stream, runner)
	r := stream.Reader()
	for tok := r.Next(); tok != nil; tok = r.Next() {
		fmt.Println(tok)
	}
	// Output: Scalar("Alice")
}
