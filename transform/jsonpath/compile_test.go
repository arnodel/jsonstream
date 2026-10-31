package jsonpath_test

import (
	"os"
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

// ExampleCompileQueryString shows the whole pipeline from Go code: decode
// JSON, run a JSONPath query over it, and pretty-print the results.
func ExampleCompileQueryString() {
	runner, err := jsonpathtransformer.CompileQueryString("$.users[?@.age > 30]")
	if err != nil {
		panic(err)
	}
	input := strings.NewReader(`{"users": [{"name": "Alice", "age": 35}, {"name": "Bob", "age": 25}]}`)
	stream := token.StartStream(json.NewDecoder(input), nil)
	stream = token.TransformStream(stream, runner)
	if err := token.ConsumeStream(stream, json.NewEncoder(os.Stdout)); err != nil {
		panic(err)
	}
	// Output: {"name": "Alice", "age": 35}
}
