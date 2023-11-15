package jsonstream

import (
	"github.com/arnodel/jsonstream/internal/jsonpath/ast"
	"github.com/arnodel/jsonstream/internal/jsonpath/parser"
)

type JsonPathQueryTransformer struct {
	query ast.Query
}

func NewJsonPathQueryTransformer(query ast.Query) *JsonPathQueryTransformer {
	return &JsonPathQueryTransformer{
		query: query,
	}
}
func (t *JsonPathQueryTransformer) Transform(in <-chan Token, out chan<- Token) {
	for _, segment := range t.query.Segments {

		segmentTransformer := AsStreamTransformer(newSegmentTransformer(segment))
		in = TransformStream(in, segmentTransformer)
	}
	for token := range in {
		out <- token
	}
}

func newSegmentTransformer(segment ast.Segment) ValueTransformer {
	switch segment.Type {
	case ast.ChildSegmentType:
		return ChildSegmentTransformer{selectors: segment.Selectors}
	case ast.DescendantSegmentType:
		return DescendantSegmentTransformer{selectors: segment.Selectors}
	default:
		panic("invalic segment")
	}
}

type ChildSegmentTransformer struct {
	selectors []ast.Selector
}

func (t ChildSegmentTransformer) TransformValue(value StreamedValue, out chan<- Token) {
	switch x := value.(type) {
	case *StreamedObject:
		for x.Advance() {
			keyScalar, value := x.CurrentKeyVal()
			key := scalarValue(keyScalar)
			selectCounts := countSelectsFromKey(t.selectors, key)
			var restart RestartFunc
			if selectCounts[DontKnow] > 0 || selectCounts[Yes] > 1 {
				restart = value.MakeRestartable()
			}
			for _, selector := range t.selectors {
				if selects(selector, key, value) {
					value.Copy(out)
				}
				if restart != nil {
					restart()
				}
			}
		}
	case *StreamedArray:
		var index int64
		for x.Advance() {
			value := x.CurrentValue()
			selectCounts := countSelectsFromKey(t.selectors, index)
			var restart RestartFunc
			if selectCounts[DontKnow] > 0 || selectCounts[Yes] > 1 {
				restart = value.MakeRestartable()
			}
			for _, selector := range t.selectors {
				if selects(selector, index, value) {
					value.Copy(out)
				}
				if restart != nil {
					restart()
				}
			}
			index++
		}
	default:
		x.Discard()
	}
}

type DescendantSegmentTransformer struct {
	selectors []ast.Selector
}

func (t DescendantSegmentTransformer) TransformValue(value StreamedValue, out chan<- Token) {
	switch x := value.(type) {
	case *StreamedObject:
		for x.Advance() {
			keyScalar, value := x.CurrentKeyVal()
			key := scalarValue(keyScalar)
			selectCounts := countSelectsFromKey(t.selectors, key)
			var restart RestartFunc
			if selectCounts[DontKnow] > 0 || selectCounts[Yes] > 0 {
				restart = value.MakeRestartable()
			}
			for _, selector := range t.selectors {
				if selects(selector, key, value) {
					value.Copy(out)
				}
				if restart != nil {
					restart()
				}
			}
			t.TransformValue(value, out)
		}
	case *StreamedArray:
		var index int64
		for x.Advance() {
			value := x.CurrentValue()
			selectCounts := countSelectsFromKey(t.selectors, index)
			var restart RestartFunc
			if selectCounts[DontKnow] > 0 || selectCounts[Yes] > 0 {
				restart = value.MakeRestartable()
			}
			for _, selector := range t.selectors {
				if selects(selector, index, value) {
					value.Copy(out)
				}
				if restart != nil {
					restart()
				}
			}
			t.TransformValue(value, out)
		}
	default:
		x.Discard()
	}

}

func countSelectsFromKey(selectors []ast.Selector, key any) (counts [3]int) {
	for _, selector := range selectors {
		counts[selectsFromKey(selector, key)]++
	}
	return
}

func selectsFromKey(selector ast.Selector, key any) Decision {
	switch x := selector.(type) {
	case ast.NameSelector:
		return madeDecision(key == x.Name)
	case ast.WildcardSelector:
		return Yes
	case ast.IndexSelector:
		keyInt, ok := key.(int64)
		return madeDecision(ok && keyInt == x.Index)
	case ast.FilterSelector:
		return DontKnow
	default:
		panic("invalid selector")
	}
}

func selects(selector ast.Selector, key any, value StreamedValue) bool {
	switch x := selector.(type) {
	case ast.NameSelector:
		return key == x.Name
	case ast.WildcardSelector:
		return true
	case ast.IndexSelector:
		keyInt, ok := key.(int64)
		return ok && keyInt == x.Index
	case ast.FilterSelector:
		panic("not implemented")
	default:
		panic("invalid selector")
	}
}

func scalarValue(scalar *Scalar) any {
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
	Evaluate(value StreamedValue) bool
}

type ComparableEvaluator interface {
	Evaluate(value StreamedValue) StreamedValue
}

type LogicalOrEvaluator struct {
	Arguments []LogicalEvaluator
}

func (e LogicalOrEvaluator) Evaluate(value StreamedValue) bool {
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

func (e LogicalAndEvaluator) Evaluate(value StreamedValue) bool {
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

func (e LogicalNotEvaluator) Evaluate(value StreamedValue) bool {
	return !e.Argument.Evaluate(value)
}

type ComparisonEvaluator struct {
	left  ComparableEvaluator
	op    ast.ComparisonOp
	right ComparableEvaluator
}

func (e ComparisonEvaluator) Evaluate(value StreamedValue) bool {
	leftValue := e.left.Evaluate(value)
	rightValue := e.right.Evaluate(value)
	return compare(leftValue, e.op, rightValue)
}

func compare(left StreamedValue, op ast.ComparisonOp, right StreamedValue) bool {
	panic("unimplemented")
}

type LiteralEvaluator struct {
	value StreamedValue
}

func (e LiteralEvaluator) Evaluate(value StreamedValue) StreamedValue {
	return e.value
}
