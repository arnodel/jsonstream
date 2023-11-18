package jsonpathtransformer

import (
	"github.com/arnodel/jsonstream/iterator"
	"github.com/arnodel/jsonstream/token"
)

//
//
//

type SelectorRunner interface {
	SelectsFromKey(key any) Decision
	Selects(key any, Value iterator.Value) bool
}

var _ SelectorRunner = NameSelectorRunner{}
var _ SelectorRunner = WildcardSelectorRunner{}
var _ SelectorRunner = IndexSelectorRunner{}
var _ SelectorRunner = SliceSelectorRunner{}
var _ SelectorRunner = NothingSelectorRunner{}

type NameSelectorRunner struct {
	name string
}

func (r NameSelectorRunner) SelectsFromKey(key any) Decision {
	return madeDecision(key == r.name)
}

func (r NameSelectorRunner) Selects(key any, value iterator.Value) bool {
	return key == r.name
}

type WildcardSelectorRunner struct{}

func (r WildcardSelectorRunner) SelectsFromKey(key any) Decision {
	return Yes
}

func (r WildcardSelectorRunner) Selects(key any, value iterator.Value) bool {
	return true
}

type IndexSelectorRunner struct {
	index int64
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

func (r SliceSelectorRunner) SelectsFromKey(key any) Decision {
	index, ok := key.(int64)
	return madeDecision(ok && index >= r.start && index < r.end && (index-r.start)%r.step == 0)
}

func (r SliceSelectorRunner) Selects(key any, value iterator.Value) bool {
	index, ok := key.(int64)
	return ok && index >= r.start && index < r.end && (index-r.start)%r.step == 0
}

type NothingSelectorRunner struct{}

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

type ChildSegmentRunner struct {
	selectors []SelectorRunner
}

func (t ChildSegmentRunner) TransformValue(value iterator.Value, out chan<- token.Token) {
	switch x := value.(type) {
	case *iterator.Object:
		for x.Advance() {
			keyScalar, value := x.CurrentKeyVal()
			key := scalarValue(keyScalar)
			selectCounts := countSelectsFromKey(t.selectors, key)
			var restart iterator.RestartFunc
			if selectCounts[DontKnow] > 0 || selectCounts[Yes] > 1 {
				restart = value.MakeRestartable()
			}
			for _, selector := range t.selectors {
				if selector.Selects(key, value) {
					value.Copy(out)
				}
				if restart != nil {
					restart()
				}
			}
		}
	case *iterator.Array:
		var index int64
		for x.Advance() {
			value := x.CurrentValue()
			selectCounts := countSelectsFromKey(t.selectors, index)
			var restart iterator.RestartFunc
			if selectCounts[DontKnow] > 0 || selectCounts[Yes] > 1 {
				restart = value.MakeRestartable()
			}
			for _, selector := range t.selectors {
				if selector.Selects(index, value) {
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

type DescendantSegmentRunner struct {
	selectors []SelectorRunner
}

func (t DescendantSegmentRunner) TransformValue(value iterator.Value, out chan<- token.Token) {
	switch x := value.(type) {
	case *iterator.Object:
		for x.Advance() {
			keyScalar, value := x.CurrentKeyVal()
			key := scalarValue(keyScalar)
			selectCounts := countSelectsFromKey(t.selectors, key)
			var restart iterator.RestartFunc
			if selectCounts[DontKnow] > 0 || selectCounts[Yes] > 0 {
				restart = value.MakeRestartable()
			}
			for _, selector := range t.selectors {
				if selector.Selects(key, value) {
					value.Copy(out)
				}
				if restart != nil {
					restart()
				}
			}
			t.TransformValue(value, out)
		}
	case *iterator.Array:
		var index int64
		for x.Advance() {
			value := x.CurrentValue()
			selectCounts := countSelectsFromKey(t.selectors, index)
			var restart iterator.RestartFunc
			if selectCounts[DontKnow] > 0 || selectCounts[Yes] > 0 {
				restart = value.MakeRestartable()
			}
			for _, selector := range t.selectors {
				if selector.Selects(index, value) {
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

func countSelectsFromKey(selectors []SelectorRunner, key any) (counts [3]int) {
	for _, selector := range selectors {
		counts[selector.SelectsFromKey(key)]++
	}
	return
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
