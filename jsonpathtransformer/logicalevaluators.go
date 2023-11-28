package jsonpathtransformer

import (
	"bytes"

	"github.com/arnodel/jsonstream/internal/jsonpath/parser"
	"github.com/arnodel/jsonstream/iterator"
	"github.com/arnodel/jsonstream/token"
)

type LogicalEvaluator interface {
	EvaluateTruth(value iterator.Value) bool
}

var _ LogicalEvaluator = LogicalOrEvaluator{}
var _ LogicalEvaluator = LogicalAndEvaluator{}
var _ LogicalEvaluator = LogicalNotEvaluator{}
var _ LogicalEvaluator = ComparisonEvaluator{}
var _ LogicalEvaluator = QueryRunner{}

type LogicalOrEvaluator struct {
	Arguments []LogicalEvaluator
}

func (e LogicalOrEvaluator) EvaluateTruth(value iterator.Value) bool {
	for _, arg := range e.Arguments {
		if arg.EvaluateTruth(value) {
			return true
		}
	}
	return false
}

type LogicalAndEvaluator struct {
	Arguments []LogicalEvaluator
}

func (e LogicalAndEvaluator) EvaluateTruth(value iterator.Value) bool {
	for _, arg := range e.Arguments {
		if !arg.EvaluateTruth(value) {
			return false
		}
	}
	return true
}

type LogicalNotEvaluator struct {
	Argument LogicalEvaluator
}

func (e LogicalNotEvaluator) EvaluateTruth(value iterator.Value) bool {
	return !e.Argument.EvaluateTruth(value)
}

type ComparisonEvaluator struct {
	left  ComparableEvaluator
	flags ComparisonFlags
	right ComparableEvaluator
}

type ComparisonFlags uint8

const (
	CheckEquals ComparisonFlags = 1 << iota
	CheckLessThan
	NegateResult
)

func (e ComparisonEvaluator) EvaluateTruth(value iterator.Value) bool {
	leftValue := e.left.Evaluate(value)
	rightValue := e.right.Evaluate(value)
	result := false
	if e.flags&CheckEquals != 0 {
		result = checkEquals(leftValue, rightValue)
	}
	if !result && e.flags&CheckLessThan != 0 {
		result = checkLessThan(leftValue, rightValue)
	}
	return result != (e.flags&NegateResult != 0)
}

func checkEquals(left iterator.Value, right iterator.Value) bool {
	if left == nil {
		return right == nil
	}
	switch x := left.(type) {
	case *iterator.Scalar:
		y, ok := right.(*iterator.Scalar)
		if !ok {
			return false
		}
		return checkScalarEquals(x.Scalar(), y.Scalar())
	case *iterator.Object:
		panic("unimplemented")
	default:
		panic("invalid ")
	}
}

func checkScalarEquals(left *token.Scalar, right *token.Scalar) bool {
	if left.Type() != right.Type() {
		return false
	}
	switch left.Type() {
	case token.Null:
		return true
	case token.Boolean:
		// The bytes are "true" or "false", so it's enough to compare the first one
		return left.Bytes[0] == right.Bytes[0]
	case token.String:
		if bytes.Equal(left.Bytes, right.Bytes) {
			return true
		}
		if left.IsUnescaped() && right.IsUnescaped() {
			return false
		}
	case token.Number:
		if bytes.Equal(left.Bytes, right.Bytes) {
			return true
		}
	default:
		panic("invalid scalar type")
	}
	// Fall back to slower conversion
	return parser.ParseJsonLiteralBytes(left.Bytes) == parser.ParseJsonLiteralBytes(right.Bytes)
}

func checkLessThan(left iterator.Value, right iterator.Value) bool {
	if left == nil || right == nil {
		return false
	}
	x, ok := left.(*iterator.Scalar)
	if !ok {
		return false
	}
	y, ok := right.(*iterator.Scalar)
	if !ok {
		return false
	}
	xs := x.Scalar()
	ys := y.Scalar()
	if xs.Type() != ys.Type() {
		return false
	}
	switch xs.Type() {
	case token.Number:
		xx := parser.ParseJsonLiteralBytes(xs.Bytes).(float64)
		yy := parser.ParseJsonLiteralBytes(xs.Bytes).(float64)
		return xx < yy
	case token.String:
		xx := parser.ParseJsonLiteralBytes(xs.Bytes).(string)
		yy := parser.ParseJsonLiteralBytes(xs.Bytes).(string)
		return xx < yy
	default:
		return false
	}
}
