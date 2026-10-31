package jsonpath

import (
	"bytes"
	"strings"

	"github.com/arnodel/jsonstream/iterator"
	"github.com/arnodel/jsonstream/token"
)

// literalOperand is a scalar literal appearing in a comparison, parsed once
// when the query is compiled rather than every time the comparison runs.
type literalOperand struct {
	scalar   *token.Scalar
	num      float64 // value when the literal is a number
	str      string  // value when the literal is a string
	strBytes []byte  // str as bytes, to compare with unescaped JSON strings
}

func newLiteralOperand(s *token.Scalar) literalOperand {
	op := literalOperand{scalar: s}
	switch s.Type() {
	case token.Number:
		op.num = s.ToFloat64()
	case token.String:
		op.str = s.ToString()
		op.strBytes = []byte(op.str)
	}
	return op
}

// equals reports whether the scalar s is equal to the literal, with the same
// semantics as token.Scalar.Equal but parsing at most one operand.
func (op literalOperand) equals(s *token.Scalar) bool {
	if s.Type() != op.scalar.Type() {
		return false
	}
	switch s.Type() {
	case token.Null:
		return true
	case token.Boolean:
		return s.Bytes[0] == op.scalar.Bytes[0]
	case token.Number:
		return bytes.Equal(s.Bytes, op.scalar.Bytes) || s.ToFloat64() == op.num
	case token.String:
		if s.IsUnescaped() {
			return bytes.Equal(s.Bytes[1:len(s.Bytes)-1], op.strBytes)
		}
		return s.ToString() == op.str
	default:
		return false
	}
}

// compare orders the scalar s against the literal, returning -1, 0 or 1 as s
// is less than, equal to or greater than it.  ok is false when they are not
// comparable: different types, or a type with no order (null, booleans).
func (op literalOperand) compare(s *token.Scalar) (c int, ok bool) {
	if s.Type() != op.scalar.Type() {
		return 0, false
	}
	switch s.Type() {
	case token.Number:
		x := s.ToFloat64()
		switch {
		case x < op.num:
			return -1, true
		case x > op.num:
			return 1, true
		default:
			return 0, true
		}
	case token.String:
		// Go compares strings bytewise, so comparing the raw bytes of an
		// unescaped JSON string gives the same order without allocating.
		if s.IsUnescaped() {
			return bytes.Compare(s.Bytes[1:len(s.Bytes)-1], op.strBytes), true
		}
		return strings.Compare(s.ToString(), op.str), true
	default:
		return 0, false
	}
}

// LiteralComparisonEvaluator is a ComparisonEvaluator for the common case
// where one operand is a literal, such as @.price > 190.  The literal is parsed
// once at compile time, and only one clone of the current node is needed.
type LiteralComparisonEvaluator struct {
	query ComparableEvaluator
	lit   literalOperand
	flags ComparisonFlags

	// literalOnLeft is true when the comparison is "literal OP query" rather
	// than "query OP literal", which matters for CheckLessThan.
	literalOnLeft bool
}

var _ LogicalEvaluator = LiteralComparisonEvaluator{}

func (e LiteralComparisonEvaluator) EvaluateTruth(ctx *RunContext, value iterator.Value) bool {
	clone, detach := value.Clone()
	if detach != nil {
		defer detach()
	}
	result := false
	// A missing node (nil) or a non-scalar node is neither equal to nor
	// ordered with a scalar literal.
	if s, ok := e.query.Evaluate(ctx, clone).(*iterator.Scalar); ok {
		scalar := s.Scalar()
		if e.flags&CheckEquals != 0 {
			result = e.lit.equals(scalar)
		}
		if !result && e.flags&CheckLessThan != 0 {
			if c, ok := e.lit.compare(scalar); ok {
				if e.literalOnLeft {
					result = c > 0 // literal < node
				} else {
					result = c < 0 // node < literal
				}
			}
		}
	}
	return result != (e.flags&NegateResult != 0)
}
