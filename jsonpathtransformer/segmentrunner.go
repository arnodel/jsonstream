package jsonpathtransformer

import (
	"math"

	"github.com/arnodel/jsonstream/internal/jsonpath/parser"
	"github.com/arnodel/jsonstream/iterator"
	"github.com/arnodel/jsonstream/token"
)

//
// Segment runner
//

// SegmentRunner implements the runner of a query segment
type SegmentRunner struct {

	// Runners for the selectors this segment is made of.
	selectors []SelectorRunner

	// The max lookahead of all its selectors (see SelectorRunner.Lookahead()
	// for details).
	lookahead int64

	// There are two kinds of segments, child or descendant.  This field is true
	// iff this is a descendant segment.
	isDescendantSegment bool
}

// TransformValue transforms the incoming value according to the definition of
// the query segment.
func (r SegmentRunner) TransformValue(value iterator.Value, next valueProcessor) {
	// We allocate decisions here because otherwise we would allocate a new
	// slice fore each item in the collection.
	//
	// Hopefully escape analysis will prove that the slice can't escape, and
	// since its capacity is known, it should be allocated on the stack.
	decisions := make([]Decision, 0, 10)
	r.transformValue(value, decisions, next)
}

func (r SegmentRunner) transformValue(value iterator.Value, decisions []Decision, next valueProcessor) {
	switch x := value.(type) {
	case *iterator.Object:
		for x.Advance() {
			keyScalar, value := x.CurrentKeyVal()
			key := keyStringValue(keyScalar)
			decisions = decisions[:0]
			for _, selector := range r.selectors {
				decisions = append(decisions, selector.SelectsFromKey(key))
			}
			r.applySelectors(value, decisions, next)
			if r.isDescendantSegment {
				r.transformValue(value, decisions, next)
			}
		}
	case *iterator.Array:
		var index, negIndex int64
		var ahead *iterator.Array

		if r.lookahead > 0 {
			var detach func()
			ahead, detach = x.CloneArray()
			defer detach()
			defer ahead.Discard()
			for negIndex+r.lookahead >= 0 && ahead.Advance() {
				negIndex--
			}
		} else {
			negIndex = math.MinInt64
		}

		for x.Advance() {
			value := x.CurrentValue()
			decisions = decisions[:0]
			for _, selector := range r.selectors {
				decisions = append(decisions, selector.SelectsFromIndex(index, negIndex))
			}
			r.applySelectors(value, decisions, next)
			index++
			if ahead != nil && !ahead.Advance() {
				negIndex++
			}
			if r.isDescendantSegment {
				r.transformValue(value, decisions, next)
			}
		}
	default:
		x.Discard()
	}
}

func (r *SegmentRunner) applySelectors(value iterator.Value, decisions []Decision, next valueProcessor) {
	// We need to count decisions which may select the value so that we know
	// when not to clone it before copying it to the output.  This may appear
	// like a small optimization but in practice almost all segments are made
	// out of 1 selector, in which case cloning the value is not needed so it's
	// woth catering for.
	perhapsCount := 0
	for _, d := range decisions {
		if d != No {
			perhapsCount++
		}
	}
	for i, selector := range r.selectors {
		switch decisions[i] {
		case DontKnow:
			perhapsCount--
			if !selector.SelectsFromValue(value) {
				continue
			}
		case Yes:
			perhapsCount--
		default:
			continue
		}
		if perhapsCount > 0 {
			clone, detach := value.Clone()
			if detach != nil {
				defer detach()
			}
			next.ProcessValue(clone)
		} else {
			next.ProcessValue(value)
		}
	}
}

// This assumes the scalar is a string - it should always be the case for an
// object key.
func keyStringValue(scalar *token.Scalar) string {
	if scalar.IsUnescaped() {
		return string(scalar.Bytes[1 : len(scalar.Bytes)-1])
	}
	return parser.ParseJsonLiteralBytes(scalar.Bytes).(string)
}
