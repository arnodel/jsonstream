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

type detachableValue struct {
	value      iterator.Value
	detachFunc func()
}

func (dv detachableValue) detach() {
	if dv.detachFunc != nil {
		dv.detachFunc()
	}
}

type selectorState struct {
	selector          SelectorRunner
	selected          bool
	pending           []detachableValue
	done              bool
	reversesSelection bool
}

func (s *selectorState) flush(ctx *RunContext, result bool, next valueProcessor) bool {
	if s.reversesSelection {
		for i := len(s.pending) - 1; i >= 0; i-- {
			dv := s.pending[i]
			if result {
				result = next.ProcessValue(ctx, dv.value)
			}
			dv.detach()
		}
	} else {
		for _, dv := range s.pending {
			if result {
				result = next.ProcessValue(ctx, dv.value)
			}
			dv.detach()
		}
	}
	s.pending = nil
	return result
}

type valueDispatcher struct {
	shouldClone    bool
	selectorStates []selectorState
	next           valueProcessor
}

func newValueDispatcher(selectors []SelectorRunner, next valueProcessor) *valueDispatcher {
	selectorStates := make([]selectorState, len(selectors))
	for i, selector := range selectors {
		selectorStates[i].selector = selector
		selectorStates[i].reversesSelection = selector.ReversesSelection()
	}
	return &valueDispatcher{
		selectorStates: selectorStates,
		next:           next,
	}
}

func (d *valueDispatcher) flush(ctx *RunContext, result bool) bool {
	for _, state := range d.selectorStates {
		result = state.flush(ctx, result, d.next)
	}
	return result
}

func (d *valueDispatcher) transformItem(ctx *RunContext, value iterator.Value, decide func(SelectorRunner) Decision, followingSegments []SegmentRunner) (result bool) {
	result = true

	if len(d.selectorStates) == 0 {
		return
	}

	// First find which selectors may apply, which will be done after
	// this round.
	selectedCount := 0
	firstLiveIndex := 0
	for i := range d.selectorStates {
		state := &d.selectorStates[i]
		if state.done {
			state.selected = false
			if firstLiveIndex == i {
				firstLiveIndex++
			}
		} else {
			selector := state.selector
			decision := decide(selector)
			state.selected = decision.IsYes() || !decision.IsNo() && selector.SelectsFromValue(ctx, value)
			if state.selected {
				selectedCount++
			}
			if decision.IsNoMoreAfter() {
				state.done = true
				if !state.selected && firstLiveIndex == i {
					firstLiveIndex++
				}
			}
		}
	}

	// Flush finished states
	if firstLiveIndex > 0 {
		for _, state := range d.selectorStates[:firstLiveIndex] {
			result = state.flush(ctx, result, d.next)
		}
		d.selectorStates = d.selectorStates[firstLiveIndex:]
		if !result || len(d.selectorStates) == 0 {
			return
		}
	}

	firstState := &d.selectorStates[0]
	// Flush the first state
	if !firstState.reversesSelection && len(firstState.pending) > 0 {
		result = firstState.flush(ctx, result, d.next)
		if !result {
			return
		}
	}

	// Process the value if selected
	if selectedCount > 0 {
		d.shouldClone = selectedCount > 1 || !d.selectorStates[0].selected
		if len(followingSegments) == 0 {
			result = d.ProcessValue(ctx, value)
		} else {
			result = followingSegments[0].transformValue2(ctx, value, d, followingSegments[1:])
		}
	}
	return
}

func (d *valueDispatcher) ProcessValue(ctx *RunContext, value iterator.Value) bool {
	// Then apply the eligible selectors, but only the first one is
	// passed to next straight away
	for i := range d.selectorStates {
		state := &d.selectorStates[i]
		if state.selected {
			// TODO: We shouldn't clone but copy, but first need to make it work
			clone, detach := cloneIf(value, d.shouldClone)
			if i == 0 && !state.reversesSelection {
				result := d.next.ProcessValue(ctx, clone)
				if detach != nil {
					detach()
				}
				if !result {
					return false
				}
			} else {
				state.pending = append(state.pending, detachableValue{clone, detach})
			}
		}
	}
	return true
}

func cloneIf(value iterator.Value, cond bool) (iterator.Value, func()) {
	if cond {
		return value.Clone()
	} else {
		return value, nil
	}
}

func (r SegmentRunner) transformValue2(ctx *RunContext, value iterator.Value, next valueProcessor, followingSegments []SegmentRunner) bool {
	switch x := value.(type) {
	case *iterator.Object:
		return r.transformObject(ctx, x, next, followingSegments)
	case *iterator.Array:
		return r.transformArray(ctx, x, next, followingSegments)
	default:
		value.Discard()
		return true
	}
}

func (r SegmentRunner) transformObject(ctx *RunContext, obj *iterator.Object, next valueProcessor, followingSegments []SegmentRunner) (result bool) {
	dispatcher := newValueDispatcher(r.selectors, next)

	defer func() { dispatcher.flush(ctx, result) }()

	for obj.Advance() {
		keyScalar, value := obj.CurrentKeyVal()
		key := keyScalar.ToString()
		result = dispatcher.transformItem(ctx, value, func(s SelectorRunner) Decision { return s.SelectsFromKey(key) }, followingSegments)
		if !result {
			return
		}

		// Lastly if this is a descendant segment, we need to dive into value
		if r.isDescendantSegment {
			result = r.transformValue2(ctx, value, next, followingSegments)
			if !result {
				return
			}
		} else if len(dispatcher.selectorStates) == 0 {
			return
		}
	}
	return true
}

func (r SegmentRunner) transformArray(ctx *RunContext, arr *iterator.Array, next valueProcessor, followingSegments []SegmentRunner) (result bool) {
	dispatcher := newValueDispatcher(r.selectors, next)

	defer func() { dispatcher.flush(ctx, result) }()

	var index, negIndex int64
	var ahead *iterator.Array

	if r.lookahead > 0 {
		var detach func()
		ahead, detach = arr.CloneArray()
		defer detach()
		for negIndex+r.lookahead >= 0 && ahead.Advance() {
			negIndex--
		}
	} else {
		negIndex = math.MinInt64
	}

	for arr.Advance() {
		value := arr.CurrentValue()

		result = dispatcher.transformItem(ctx, value, func(s SelectorRunner) Decision { return s.SelectsFromIndex(index, negIndex) }, followingSegments)
		if !result {
			return
		}

		// Update the index
		index++
		if ahead != nil && !ahead.Advance() {
			negIndex++
		}

		// Lastly if this is a descendant segment, we need to dive into value
		if r.isDescendantSegment {
			result = r.transformValue2(ctx, value, next, followingSegments)
			if !result {
				return
			}
		} else if len(dispatcher.selectorStates) == 0 {
			return
		}
	}

	return true
}
