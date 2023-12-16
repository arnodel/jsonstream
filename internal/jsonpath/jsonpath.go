package jsonpath

import (
	"errors"

	"github.com/arnodel/grammar"
	"github.com/arnodel/jsonstream/internal/jsonpath/ast"
	"github.com/arnodel/jsonstream/internal/jsonpath/parser"
)

func ParseQueryString(s string) (ast.Query, error) {
	stream, err := parser.TokeniseJsonPathString(s)
	if err != nil {
		return ast.Query{}, err
	}

	var query parser.Query
	parseErr := grammar.Parse(&query, stream)
	if parseErr != nil {
		return ast.Query{}, err
	}
	if n := stream.Next(); n != grammar.EOF {
		return ast.Query{}, errors.New("invalid query string")
	}
	return query.CompileToQuery()
}
