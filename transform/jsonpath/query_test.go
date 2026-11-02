package jsonpath_test

import (
	"strings"
	"testing"

	"github.com/arnodel/jsonstream/encoding/json"
	"github.com/arnodel/jsonstream/internal/jsonpath"
	"github.com/arnodel/jsonstream/token"
	jsonpathtransformer "github.com/arnodel/jsonstream/transform/jsonpath"
)

func runSimpleTest(t *testing.T, input, query, output string) {
	src := streamJsonString(input)
	runner, err := compileQueryString(query)
	if err != nil {
		t.Fatalf("Invalid query: %s", err)
	}
	res := token.TransformStream(src, runner).Reader()

	exp := streamJsonString(output).Reader()

	for expected := exp.Next(); expected != nil; expected = exp.Next() {
		got := res.Next()
		if got == nil {
			t.Fatalf("Expected %s, got nil", expected)
		}
		if got.String() != expected.String() {
			t.Fatalf("Expected %s, got %s", expected, got)
		}
	}
	if got := res.Next(); got != nil {
		t.Fatalf("Expected end of results, got %s", got)
	}
}

func compileQueryString(s string) (jsonpathtransformer.MainQueryRunner, error) {
	query, err := jsonpath.ParseQueryString(s)
	if err != nil {
		return jsonpathtransformer.MainQueryRunner{}, err
	}
	return jsonpathtransformer.CompileQuery(query)
}

func compileQueryStringStrict(s string) (jsonpathtransformer.MainQueryRunner, error) {
	query, err := jsonpath.ParseQueryStringStrict(s)
	if err != nil {
		return jsonpathtransformer.MainQueryRunner{}, err
	}
	return jsonpathtransformer.CompileQuery(query, jsonpathtransformer.WithStrictMode(true))
}

func streamJsonString(s string) token.Stream {
	decoder := json.NewDecoder(strings.NewReader(s))
	return token.StartStream(decoder, nil)
}

func TestSimpleQueries(t *testing.T) {
	type testCase struct {
		name   string
		input  string
		query  string
		output string
	}
	var testCases = []testCase{
		{
			name:   "wildcard array",
			input:  `[1, 2, 3]`,
			query:  `$[*]`,
			output: `1 2 3`,
		},
		{
			name:   "wildcard object",
			input:  `{"x":-2, "y": 3}`,
			query:  `$[*]`,
			output: `-2 3`,
		},
		{
			name:   "just dollar",
			input:  `[1, 2]`,
			query:  `$`,
			output: `[1, 2]`,
		},
		// A reversed slice keeps the items it selects to emit them in
		// reverse, so they must be cloned even when nothing else selects them.
		{
			name:   "reversed slice of collections",
			input:  `[[1], {"a": 2}, [3, [4]]]`,
			query:  `$[::-1]`,
			output: `[3, [4]] {"a": 2} [1]`,
		},
		{
			name:   "reversed slice then index",
			input:  `[[1], {"a": 2}, [3, [4]]]`,
			query:  `$[::-1][0]`,
			output: `3 1`,
		},
		// Index queries
		{
			name:   "index",
			input:  `[1, 2, 3 , 4, 5]`,
			query:  `$[3]`,
			output: `4`,
		},
		{
			name:   "negative index",
			input:  `[1, 2, 3, 4, 5]`,
			query:  `$[-2]`,
			output: `4`,
		},
		{
			name:   "index list",
			input:  `[1, 2, 3, 4 , 5, 6 ,7, 8, 9, 10]`,
			query:  `$[1, -2, 8, -5]`,
			output: `2 9 9 6`,
		},
		// Slice queries
		{
			name:   "prefix slice",
			input:  `[1, 2, 3, 4 , 5, 6 ,7, 8, 9, 10]`,
			query:  `$[:3]`,
			output: `1 2 3`,
		},
		{
			name:   "suffix slice",
			input:  `[1, 2, 3, 4 , 5, 6 ,7, 8, 9, 10]`,
			query:  `$[-3:]`,
			output: `8 9 10`,
		},
		{
			name:   "middle slice",
			input:  `[1, 2, 3, 4 , 5, 6 ,7, 8, 9, 10]`,
			query:  `$[2:5]`,
			output: `3 4 5`,
		},
		{
			name:   "step slice",
			input:  `[1, 2, 3, 4 , 5, 6 ,7, 8, 9, 10]`,
			query:  `$[::3]`,
			output: `1 4 7 10`,
		},
		{
			name:   "suffix step slice",
			input:  `[1, 2, 3, 4 , 5, 6 ,7, 8, 9, 10]`,
			query:  `$[2::3]`,
			output: `3 6 9`,
		},
		{
			name:   "prefix step slice",
			input:  `[1, 2, 3, 4 , 5, 6 ,7, 8, 9, 10]`,
			query:  `$[:6:3]`,
			output: `1 4`,
		},
		{
			name:   "middle step slice",
			input:  `[1, 2, 3, 4 , 5, 6 ,7, 8, 9, 10]`,
			query:  `$[1:8:3]`,
			output: `2 5 8`,
		},
		// Child queries
		{
			name:   "child query dot syntax",
			input:  `{"a": {"b": 1}, "b": {"a": 2}}`,
			query:  `$.a.b`,
			output: `1`,
		},
		{
			name:   "child query bracket syntax",
			input:  `{"a": {"b": 1}, "b": {"a": 2}}`,
			query:  `$['b']["a"]`,
			output: `2`,
		},
		// Descendant queries
		{
			name:   "descendant query dot syntax",
			input:  `{"a": {"b": 1}, "b": {"a": 2}}`,
			query:  `$..a`,
			output: `{"b": 1} 2`,
		},
		{
			name:   "descendant query bracket syntax",
			input:  `{"a": {"b": 1}, "b": {"a": 2}}`,
			query:  `$..["b"]`,
			output: `1 {"a": 2}`,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			runSimpleTest(t, c.input, c.query, c.output)
		})
	}
}
