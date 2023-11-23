package jsonpathtransformer

import (
	"math"

	"github.com/arnodel/jsonstream/internal/jsonpath/parser"
	"github.com/arnodel/jsonstream/iterator"
	"github.com/arnodel/jsonstream/token"
)

//
//
//

type SelectorRunner interface {
	Lookahead() int64
	SelectsFromKey(key string) Decision
	SelectsFromIndex(index, negIndex int64) Decision

	// Selects promises not to advance Value, so it must clone it first if it wants
	// to look inside.
	SelectsFromValue(value iterator.Value) bool
}

var _ SelectorRunner = NameSelectorRunner{}
var _ SelectorRunner = WildcardSelectorRunner{}
var _ SelectorRunner = IndexSelectorRunner{}
var _ SelectorRunner = SliceSelectorRunner{}
var _ SelectorRunner = NothingSelectorRunner{}

type NothingSelectorRunner struct{}

func (r NothingSelectorRunner) Lookahead() int64 {
	return 0
}

func (r NothingSelectorRunner) SelectsFromIndex(index, negIndex int64) Decision {
	return No
}

func (r NothingSelectorRunner) SelectsFromKey(key string) Decision {
	return No
}

func (r NothingSelectorRunner) SelectsFromValue(value iterator.Value) bool {
	return false
}

type NameSelectorRunner struct {
	NothingSelectorRunner
	name string
}

func (r NameSelectorRunner) SelectsFromKey(key string) Decision {
	return madeDecision(key == r.name)
}

type WildcardSelectorRunner struct {
	NothingSelectorRunner
}

func (r WildcardSelectorRunner) SelectsFromIndex(index, negIndex int64) Decision {
	return Yes
}

func (r WildcardSelectorRunner) SelectsFromKey(key string) Decision {
	return Yes
}

type IndexSelectorRunner struct {
	NothingSelectorRunner
	index int64
}

func (r IndexSelectorRunner) SelectsFromIndex(index, negIndex int64) Decision {
	if r.index >= 0 {
		return madeDecision(index == r.index)
	} else {
		return madeDecision(negIndex == r.index)
	}
}

func (r IndexSelectorRunner) Lookahead() int64 {
	if r.index < 0 {
		return -r.index
	}
	return 0
}

type SliceSelectorRunner struct {
	NothingSelectorRunner
	start, end, step int64
}

func (r SliceSelectorRunner) Lookahead() int64 {
	// For now negative step is unsupported, so lookahead is only for negative
	// start and end
	// max(-r.start, -r.end, 0)
	lookahead := -r.start
	if -r.end > lookahead {
		lookahead = -r.end
	}
	if lookahead > 0 {
		return lookahead
	}
	return 0
}

func (r SliceSelectorRunner) SelectsFromIndex(index, negIndex int64) Decision {
	var startOffset, endOffset int64
	if r.start < 0 {
		startOffset = negIndex - r.start
	} else {
		startOffset = index - r.start
	}
	if r.end < 0 {
		endOffset = negIndex - r.end
	} else {
		endOffset = index - r.end
	}
	return madeDecision(startOffset >= 0 && endOffset < 0 && startOffset%r.step == 0)
}

//
//
//

type SegmentRunner struct {
	selectors           []SelectorRunner
	lookahead           int64
	isDescendantSegment bool
}

func (r SegmentRunner) TransformValue(value iterator.Value, out chan<- token.Token) {
	// We allocate decisions here because otherwise we would allocate a new
	// slice fore each item in the collection.
	//
	// Hopefully escape analysis will prove that the slice can't escape, and
	// since its capacity is known, it should be allocated on the stack.  I
	// don't know if that will work because transformValue is recursive.
	decisions := make([]Decision, 0, 10)
	r.transformValue(value, decisions, out)
}

func (r SegmentRunner) transformValue(value iterator.Value, decisions []Decision, out chan<- token.Token) {
	switch x := value.(type) {
	case *iterator.Object:
		for x.Advance() {
			keyScalar, value := x.CurrentKeyVal()
			key := keyStringValue(keyScalar)
			decisions = r.applyKeySelectors(key, value, decisions, out)
			if r.isDescendantSegment {
				r.transformValue(value, decisions, out)
			}
		}
	case *iterator.Array:
		var index, negIndex int64
		var ahead *iterator.Array

		if r.lookahead > 0 {
			ahead = x.CloneArray()
			for negIndex+r.lookahead >= 0 && ahead.Advance() {
				negIndex--
			}
		} else {
			negIndex = math.MinInt64
		}

		for x.Advance() {
			value := x.CurrentValue()
			decisions = r.applyIndexSelectors(index, negIndex, value, decisions, out)
			index++
			if ahead != nil && !ahead.Advance() {
				negIndex++
			}
			if r.isDescendantSegment {
				r.transformValue(value, decisions, out)
			}
		}
	default:
		x.Discard()
	}
}

func (r *SegmentRunner) applyKeySelectors(key string, value iterator.Value, decisions []Decision, out chan<- token.Token) []Decision {
	var selectCounts [3]int
	decisions, selectCounts = countSelectsFromKey(r.selectors, key, decisions[:0])
	perhapsCount := selectCounts[Yes] + selectCounts[DontKnow]
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
			value.Clone().Copy(out)
		} else {
			value.Copy(out)
		}
	}
	return decisions
}

func (r *SegmentRunner) applyIndexSelectors(index int64, negIndex int64, value iterator.Value, decisions []Decision, out chan<- token.Token) []Decision {
	var selectCounts [3]int
	decisions, selectCounts = countSelectsFromIndex(r.selectors, index, negIndex, decisions[:0])
	perhapsCount := selectCounts[Yes] + selectCounts[DontKnow]
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
			value.Clone().Copy(out)
		} else {
			value.Copy(out)
		}
	}
	return decisions
}

func countSelectsFromKey(selectors []SelectorRunner, key string, dest []Decision) ([]Decision, [3]int) {
	var counts [3]int
	for _, selector := range selectors {
		decision := selector.SelectsFromKey(key)
		dest = append(dest, decision)
		counts[decision]++
	}
	return dest, counts
}

func countSelectsFromIndex(selectors []SelectorRunner, index int64, aheadIndex int64, dest []Decision) ([]Decision, [3]int) {
	var counts [3]int
	for _, selector := range selectors {
		decision := selector.SelectsFromIndex(index, aheadIndex)
		dest = append(dest, decision)
		counts[decision]++
	}
	return dest, counts
}

// This assumes the scalar is a string
func keyStringValue(scalar *token.Scalar) string {
	if scalar.IsUnescaped() {
		return string(scalar.Bytes[1 : len(scalar.Bytes)-1])
	}
	return parser.ParseJsonLiteralBytes(scalar.Bytes).(string)
}

//
//
//

type QueryRunner interface {
	token.StreamTransformer
}

type RootNodeQueryRunner struct {
	segments []SegmentRunner
}

func (r RootNodeQueryRunner) Transform(in <-chan token.Token, out chan<- token.Token) {
	for _, segment := range r.segments {
		segmentTransformer := iterator.AsStreamTransformer(segment)
		in = token.TransformStream(in, segmentTransformer)
	}
	for token := range in {
		out <- token
	}
}
