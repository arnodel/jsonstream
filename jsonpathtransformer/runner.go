package jsonpathtransformer

import (
	"github.com/arnodel/jsonstream/iterator"
	"github.com/arnodel/jsonstream/token"
)

//
//
//

type SelectorRunner interface {
	Lookahead() int64
	SelectsFromKey(key any) Decision

	// Selects promises not to advance Value, so it must clone it first if it wants
	// to look inside.
	Selects(key any, Value iterator.Value) bool
}

var _ SelectorRunner = NameSelectorRunner{}
var _ SelectorRunner = WildcardSelectorRunner{}
var _ SelectorRunner = IndexSelectorRunner{}
var _ SelectorRunner = SliceSelectorRunner{}
var _ SelectorRunner = NothingSelectorRunner{}

type NoLookaheadSelector struct{}

func (s NoLookaheadSelector) Lookahead() int64 {
	return 0
}

type NameSelectorRunner struct {
	NoLookaheadSelector
	name string
}

func (r NameSelectorRunner) SelectsFromKey(key any) Decision {
	return madeDecision(key == r.name)
}

func (r NameSelectorRunner) Selects(key any, value iterator.Value) bool {
	return key == r.name
}

type WildcardSelectorRunner struct {
	NoLookaheadSelector
}

func (r WildcardSelectorRunner) SelectsFromKey(key any) Decision {
	return Yes
}

func (r WildcardSelectorRunner) Selects(key any, value iterator.Value) bool {
	return true
}

type IndexSelectorRunner struct {
	index int64
}

func (r IndexSelectorRunner) Lookahead() int64 {
	if r.index < 0 {
		return r.index
	}
	return 0
}

func (r IndexSelectorRunner) SelectsFromKey(key any) Decision {
	return madeDecision(key == r.index)
}

func (r IndexSelectorRunner) Selects(key any, value iterator.Value) bool {
	return key == r.index
}

type SliceSelectorRunner struct {
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

func (r SliceSelectorRunner) SelectsFromKey(key any) Decision {
	index, ok := key.(int64)
	return madeDecision(ok && index >= r.start && index < r.end && (index-r.start)%r.step == 0)
}

func (r SliceSelectorRunner) Selects(key any, value iterator.Value) bool {
	index, ok := key.(int64)
	return ok && index >= r.start && index < r.end && (index-r.start)%r.step == 0
}

type NothingSelectorRunner struct {
	NoLookaheadSelector
}

func (r NothingSelectorRunner) SelectsFromKey(key any) Decision {
	return No
}

func (r NothingSelectorRunner) Selects(key any, value iterator.Value) bool {
	return false
}

//
//
//

type SegmentRunner interface {
	iterator.ValueTransformer
}

type segmentRunnerBase struct {
	selectors []SelectorRunner
	lookahead int64
}

func (r *segmentRunnerBase) applySelectors(key any, value iterator.Value, decisions []Decision, out chan<- token.Token) []Decision {
	var selectCounts [3]int
	decisions, selectCounts = countSelectsFromKey(r.selectors, key, decisions[:0])
	perhapsCount := selectCounts[Yes] + selectCounts[DontKnow]
	for i, selector := range r.selectors {
		switch decisions[i] {
		case DontKnow:
			perhapsCount--
			if !selector.Selects(key, value) {
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

type ChildSegmentRunner struct {
	segmentRunnerBase
}

func (t ChildSegmentRunner) TransformValue(value iterator.Value, out chan<- token.Token) {
	// We allocate decisions here because otherwise we would allocate a new
	// slice fore each item in the collection.
	//
	// Hopefully escape analysis will prove that the slice can't escape, and
	// since its capacity is known, it should be allocated on the stack.
	decisions := make([]Decision, 0, 10)
	switch x := value.(type) {
	case *iterator.Object:
		for x.Advance() {
			keyScalar, value := x.CurrentKeyVal()
			key := scalarValue(keyScalar)
			decisions = t.applySelectors(key, value, decisions, out)
		}
	case *iterator.Array:
		var index int64
		for x.Advance() {
			value := x.CurrentValue()
			decisions = t.applySelectors(index, value, decisions, out)
			index++
		}
	default:
		x.Discard()
	}
}

type DescendantSegmentRunner struct {
	segmentRunnerBase
}

func (t DescendantSegmentRunner) TransformValue(value iterator.Value, out chan<- token.Token) {
	// We allocate decisions here because otherwise we would allocate a new
	// slice fore each item in the collection.
	//
	// Hopefully escape analysis will prove that the slice can't escape, and
	// since its capacity is known, it should be allocated on the stack.  I
	// don't know if that will work because transformValue is recursive.
	decisions := make([]Decision, 0, 10)
	t.transformValue(value, decisions, out)
}

func (t DescendantSegmentRunner) transformValue(value iterator.Value, decisions []Decision, out chan<- token.Token) {
	switch x := value.(type) {
	case *iterator.Object:
		for x.Advance() {
			keyScalar, value := x.CurrentKeyVal()
			key := scalarValue(keyScalar)
			decisions = t.applySelectors(key, value, decisions, out)
			t.transformValue(value, decisions, out)
		}
	case *iterator.Array:
		var index int64
		for x.Advance() {
			value := x.CurrentValue()
			decisions = t.applySelectors(index, value, decisions, out)
			t.transformValue(value, decisions, out)
		}
	default:
		x.Discard()
	}

}

func countSelectsFromKey(selectors []SelectorRunner, key any, dest []Decision) ([]Decision, [3]int) {
	var counts [3]int
	for _, selector := range selectors {
		decision := selector.SelectsFromKey(key)
		dest = append(dest, decision)
		counts[decision]++
	}
	return dest, counts
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
