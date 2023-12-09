package jsonpathtransformer

import (
	"bytes"
	"slices"

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
var _ LogicalEvaluator = QueryRunner{}
var _ LogicalEvaluator = InnerQueryRunner{}

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

	rightValue := e.right.Evaluate(ctx, value2)
	result := false
	if e.flags&CheckEquals != 0 {
		result = checkEquals(leftValue, rightValue)
	}
	if !result && e.flags&CheckLessThan != 0 {
		result = checkLessThan(leftValue, rightValue)
	}
	return result != (e.flags&NegateResult != 0)
}

// This does advance the arguments
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
		y, ok := right.(*iterator.Object)
		if !ok {
			return false
		}
		return checkObjectEquals(x, y)
	case *iterator.Array:
		y, ok := right.(*iterator.Array)
		if !ok {
			return false
		}
		return checkArrayEquals(x, y)
	default:
		panic("invalid value")
	}
}

// This doesn't advance the arguments
func safeCheckEquals(left iterator.Value, right iterator.Value) bool {
	// We could have a quick path for when left and right are scalars
	val1, detach1 := left.Clone()
	val2, detach2 := right.Clone()
	if detach1 != nil {
		defer detach1()
	}
	if detach2 != nil {
		defer detach2()
	}
	return checkEquals(val1, val2)
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

func checkObjectEquals(left *iterator.Object, right *iterator.Object) bool {
	// Currently optimised for the case when the number of keys is small or the
	// keys are in a very similar order and the keys are unescaped because it
	// makes the implementation simple.  It's also probably good enough for many
	// cases, but can be very slow if both objects have many keys and they are
	// in very different orders.

	type kvPair struct {
		key    *token.Scalar
		val    iterator.Value
		detach func()
	}

	var pending []kvPair // Stores key-values in right which haven't been matched yet

	defer func() {
		for _, p := range pending {
			p.detach()
		}
	}()

iterateLeft:
	for left.Advance() {
		key, val := left.CurrentKeyVal()
		for i, p := range pending {
			if !checkScalarEquals(p.key, key) {
				continue
			}
			if !safeCheckEquals(p.val, val) {
				return false
			}
			// We have matched the pending item with the current item from left.
			if p.detach != nil {
				p.detach()
			}
			pending = slices.Delete(pending, i, i+1)
			continue iterateLeft
		}
		// Not found in pending, so consume right until we find it
		for right.Advance() {
			keyRight, valRight := right.CurrentKeyVal()

			// If the key is not the one we want, store the key-value in pending
			// items
			if !checkScalarEquals(keyRight, key) {
				valRightClone, detach := valRight.Clone()
				pending = append(pending, kvPair{keyRight, valRightClone, detach})
				continue
			}
			if !safeCheckEquals(valRight, val) {
				return false
			}
			// We have matched!
			continue iterateLeft
		}
		// At this point, we have consumed the whole of right and not found a
		// matching key.
		return false
	}
	// The objects are equal if right has no more items
	return len(pending) == 0 && !right.Advance()
}

func checkArrayEquals(left *iterator.Array, right *iterator.Array) bool {
	for left.Advance() {
		if !right.Advance() {
			return false
		}
		if !checkEquals(left.CurrentValue(), right.CurrentValue()) {
			return false
		}
	}
	// The arrays are equal if right has no more items.
	return !right.Advance()
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
