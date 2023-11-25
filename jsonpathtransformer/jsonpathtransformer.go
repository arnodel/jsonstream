package jsonpathtransformer

import (
	"github.com/arnodel/jsonstream/internal/jsonpath/ast"
)

func NewJsonPathQueryTransformer(query ast.Query) QueryRunner {
	var c Compiler
	return c.CompileQuery(query)
}
