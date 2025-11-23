package jsonpath

import (
	"github.com/arnodel/jsonstream/internal/jsonpath/parser"
	"github.com/arnodel/jsonstream/iterator"
	"github.com/arnodel/jsonstream/token"
)

type LogicalEvaluator interface {

	// EvaluateTruth returns true if the value fulfils the condition.
	// It should keep value untouched.
	EvaluateTruth(ctx *RunContext, value iterator.Value) bool
}

var _ LogicalEvaluator = LogicalOrEvaluator{}
var _ LogicalEvaluator = LogicalAndEvaluator{}
var _ LogicalEvaluator = LogicalNotEvaluator{}
var _ LogicalEvaluator = ComparisonEvaluator{}
var _ LogicalEvaluator = QueryEvaluator{}

type LogicalOrEvaluator struct {
	Arguments []LogicalEvaluator
}

func (e LogicalOrEvaluator) EvaluateTruth(ctx *RunContext, value iterator.Value) bool {
	for _, arg := range e.Arguments {
		if arg.EvaluateTruth(ctx, value) {
			return true
		}
	}
	return false
}

type LogicalAndEvaluator struct {
	Arguments []LogicalEvaluator
}

func (e LogicalAndEvaluator) EvaluateTruth(ctx *RunContext, value iterator.Value) bool {
	for _, arg := range e.Arguments {
		if !arg.EvaluateTruth(ctx, value) {
			return false
		}
	}
	return true
}

type LogicalNotEvaluator struct {
	Argument LogicalEvaluator
}

func (e LogicalNotEvaluator) EvaluateTruth(ctx *RunContext, value iterator.Value) bool {
	return !e.Argument.EvaluateTruth(ctx, value)
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

func (e ComparisonEvaluator) EvaluateTruth(ctx *RunContext, value iterator.Value) bool {
	value1, detach1 := value.Clone()
	value2, detach2 := value.Clone()
	if detach1 != nil {
		defer detach1()
	}
	if detach2 != nil {
		defer detach2()
	}
	leftValue := e.left.Evaluate(ctx, value1)
	if leftValue != nil {
		var detach func()
		leftValue, detach = leftValue.Clone()
		if detach != nil {
			defer detach()
		}
	}
	rightValue := e.right.Evaluate(ctx, value2)
	if rightValue != nil {
		var detach func()
		rightValue, detach = rightValue.Clone()
		if detach != nil {
			defer detach()
		}
	}

	result := false
	if e.flags&CheckEquals != 0 {
		result = checkEqual(leftValue, rightValue)
	}
	if !result && e.flags&CheckLessThan != 0 {
		result = checkLessThan(leftValue, rightValue)
	}
	return result != (e.flags&NegateResult != 0)
}

func checkEqual(left iterator.Value, right iterator.Value) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(right)
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
		yy := parser.ParseJsonLiteralBytes(ys.Bytes).(float64)
		return xx < yy
	case token.String:
		xx := parser.ParseJsonLiteralBytes(xs.Bytes).(string)
		yy := parser.ParseJsonLiteralBytes(ys.Bytes).(string)
		return xx < yy
	default:
		return false
	}
}
