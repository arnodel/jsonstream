package jsonpathtransformer

import (
	"math"

	"github.com/arnodel/jsonstream/iterator"
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
// the query segment.  Returns true if the processing was cancelled
func (r SegmentRunner) TransformValue(ctx *RunContext, value iterator.Value, next valueProcessor) bool {
	// We allocate decisions here because otherwise we would allocate a new
	// slice fore each item in the collection.
	//
	// Hopefully escape analysis will prove that the slice can't escape, and
	// since its capacity is known, it should be allocated on the stack.
	decisions := make([]Decision, 0, 10)
	return r.transformValue(ctx, value, decisions, next)
}

func (r SegmentRunner) transformValue(ctx *RunContext, value iterator.Value, decisions []Decision, next valueProcessor) bool {
	switch x := value.(type) {
	case *iterator.Object:
		for x.Advance() {
			keyScalar, value := x.CurrentKeyVal()
			key := keyScalar.ToString()
			decisions = decisions[:0]
			for _, selector := range r.selectors {
				decisions = append(decisions, selector.SelectsFromKey(key))
			}
			if !r.applySelectors(ctx, value, decisions, next) {
				return false
			}
			if r.isDescendantSegment && !r.transformValue(ctx, value, decisions, next) {
				return false
			}
		}
	case *iterator.Array:
		var index, negIndex int64
		var ahead *iterator.Array

		if r.lookahead > 0 {
			var detach func()
			ahead, detach = x.CloneArray()
			defer detach()
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
			if !r.applySelectors(ctx, value, decisions, next) {
				return false
			}
			index++
			if ahead != nil && !ahead.Advance() {
				negIndex++
			}
			if r.isDescendantSegment && !r.transformValue(ctx, value, decisions, next) {
				return false
			}
		}
	default:
		x.Discard()
	}
	return true
}

func (r *SegmentRunner) applySelectors(ctx *RunContext, value iterator.Value, decisions []Decision, next valueProcessor) bool {
	// We need to count decisions which may select the value so that we know
	// when not to clone it before copying it to the output.  This may appear
	// like a small optimization but in practice almost all segments are made
	// out of 1 selector, in which case cloning the value is not needed so it's
	// worth catering for.
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
			if !selector.SelectsFromValue(ctx, value) {
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
			if !next.ProcessValue(ctx, clone) {
				return false
			}
		} else {
			if !next.ProcessValue(ctx, value) {
				return false
			}
		}
	}
	return true
}
