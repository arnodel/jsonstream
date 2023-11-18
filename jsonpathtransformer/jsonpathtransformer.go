package jsonpathtransformer

import (
	"github.com/arnodel/jsonstream/internal/jsonpath/ast"
	"github.com/arnodel/jsonstream/internal/jsonpath/parser"
	"github.com/arnodel/jsonstream/iterator"
	"github.com/arnodel/jsonstream/token"
)

func NewJsonPathQueryTransformer(query ast.Query) QueryRunner {
	var c Compiler
	return c.CompileQuery(query)
}

func scalarValue(scalar *token.Scalar) any {
	if scalar.IsUnescaped() {
		return string(scalar.Bytes[1 : len(scalar.Bytes)-1])
	}
	return parser.ParseJsonLiteral(string(scalar.Bytes))
}

type Decision uint8

func (d Decision) IsMade() bool {
	return d != DontKnow
}

func (d Decision) Bool() bool {
	return d == Yes
}

func madeDecision(yes bool) Decision {
	if yes {
		return Yes
	} else {
		return No
	}
}

const (
	DontKnow Decision = 0
	Yes      Decision = 1
	No       Decision = 2
)

type LogicalEvaluator interface {
	Evaluate(value iterator.Value) bool
}

type ComparableEvaluator interface {
	Evaluate(value iterator.Value) iterator.Value
}

type LogicalOrEvaluator struct {
	Arguments []LogicalEvaluator
}

func (e LogicalOrEvaluator) Evaluate(value iterator.Value) bool {
	for _, arg := range e.Arguments {
		if arg.Evaluate(value) {
			return true
		}
	}
	return false
}

type LogicalAndEvaluator struct {
	Arguments []LogicalEvaluator
}

func (e LogicalAndEvaluator) Evaluate(value iterator.Value) bool {
	for _, arg := range e.Arguments {
		if !arg.Evaluate(value) {
			return false
		}
	}
	return true
}

type LogicalNotEvaluator struct {
	Argument LogicalEvaluator
}

func (e LogicalNotEvaluator) Evaluate(value iterator.Value) bool {
	return !e.Argument.Evaluate(value)
}

type ComparisonEvaluator struct {
	left  ComparableEvaluator
	op    ast.ComparisonOp
	right ComparableEvaluator
}

func (e ComparisonEvaluator) Evaluate(value iterator.Value) bool {
	leftValue := e.left.Evaluate(value)
	rightValue := e.right.Evaluate(value)
	return compare(leftValue, e.op, rightValue)
}

func compare(left iterator.Value, op ast.ComparisonOp, right iterator.Value) bool {
	panic("unimplemented")
}

type LiteralEvaluator struct {
	value iterator.Value
}

func (e LiteralEvaluator) Evaluate(value iterator.Value) iterator.Value {
	return e.value
}
