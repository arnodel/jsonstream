package jsonpath

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

	strictMode bool

	followingSegment *SegmentRunner
}

func (r SegmentRunner) transformValue(ctx *RunContext, value iterator.Value, next valueProcessor) bool {
	switch x := value.(type) {
	case *iterator.Object:
		if r.strictMode {
			return r.transformObjectStrict(ctx, x, next)
		} else {
			return r.transformObjectRelaxed(ctx, x, next)
		}
	case *iterator.Array:
		if r.strictMode {
			return r.transformArrayStrict(ctx, x, next)
		} else {
			return r.transformArrayRelaxed(ctx, x, next)
		}
	default:
		value.Discard()
		return true
	}
}

func (r SegmentRunner) transformObjectRelaxed(ctx *RunContext, obj *iterator.Object, next valueProcessor) (result bool) {
	dispatcher := newItemDispatcher(r.selectors, next)

	defer func() { dispatcher.flush(ctx, result) }()

	for obj.Advance() {
		keyScalar, value := obj.CurrentKeyVal()

		var value2 iterator.Value
		var detach func()

		if r.isDescendantSegment {
			value2, detach = value.Clone()
		}

		key := keyScalar.ToString()
		result = dispatcher.dispatchItem(ctx, value, func(s SelectorRunner) Decision { return s.SelectsFromKey(key) }, r.followingSegment)
		if !result {
			if detach != nil {
				detach()
			}
			return
		}

		// Lastly if this is a descendant segment, we need to dive into value
		if r.isDescendantSegment {
			result = r.transformValue(ctx, value2, next)
			if detach != nil {
				detach()
			}

			if !result {
				return
			}
		} else if len(dispatcher.selectorStates) == 0 {
			return
		}
	}
	return true
}

func (r SegmentRunner) transformArrayRelaxed(ctx *RunContext, arr *iterator.Array, next valueProcessor) (result bool) {
	dispatcher := newItemDispatcher(r.selectors, next)

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
		// negIndex has to be greater than math.MinInt64 because a reverse slice
		// with default end has an end set to math.MinInt64 and than has to be
		// beyound any possible value of negIndex.
		negIndex = math.MinInt64 + 1
	}

	for arr.Advance() {
		value := arr.CurrentValue()

		var value2 iterator.Value
		var detach func()

		if r.isDescendantSegment {
			value2, detach = value.Clone()
		}

		result = dispatcher.dispatchItem(ctx, value, func(s SelectorRunner) Decision { return s.SelectsFromIndex(index, negIndex) }, r.followingSegment)
		if !result {
			if detach != nil {
				detach()
			}
			return
		}

		// Update the index
		index++
		if ahead != nil && !ahead.Advance() {
			negIndex++
		}

		// Lastly if this is a descendant segment, we need to dive into value
		if r.isDescendantSegment {
			result = r.transformValue(ctx, value2, next)
			if detach != nil {
				detach()
			}
			if !result {
				return
			}
		} else if len(dispatcher.selectorStates) == 0 {
			return
		}
	}

	return true
}

func (r SegmentRunner) transformObjectStrict(ctx *RunContext, obj *iterator.Object, next valueProcessor) (result bool) {
	dispatcher := newItemDispatcher(r.selectors, next)

	defer func() { dispatcher.flush(ctx, result) }()

	var obj2 *iterator.Object
	var detach2 func()

	if r.isDescendantSegment {
		obj2, detach2 = obj.CloneObject()
		defer detach2()
	}

	for obj.Advance() {
		keyScalar, value := obj.CurrentKeyVal()
		key := keyScalar.ToString()
		result = dispatcher.dispatchItem(ctx, value, func(s SelectorRunner) Decision { return s.SelectsFromKey(key) }, r.followingSegment)
		if !result {
			return
		}

		if len(dispatcher.selectorStates) == 0 {
			break
		}
	}

	// Lastly if this is a descendant segment, we need to dive into values
	//
	// We do this here to comply with the order defined in the RFC but in a
	// streaming context it would be better to dive in inside the loop above.
	// Perhaps there could be a switch to decide whether we do this efficiently
	// or in a standard-compliant way.
	if r.isDescendantSegment {
		for obj2.Advance() {
			result = r.transformValue(ctx, obj2.CurrentValue(), next)
			if !result {
				return
			}
		}
	}
	return true
}

func (r SegmentRunner) transformArrayStrict(ctx *RunContext, arr *iterator.Array, next valueProcessor) (result bool) {
	dispatcher := newItemDispatcher(r.selectors, next)

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
		// negIndex has to be greater than math.MinInt64 because a reverse slice
		// with default end has an end set to math.MinInt64 and than has to be
		// beyound any possible value of negIndex.
		negIndex = math.MinInt64 + 1
	}

	var arr2 *iterator.Array
	var detach2 func()

	if r.isDescendantSegment {
		arr2, detach2 = arr.CloneArray()
		defer detach2()
	}
	for arr.Advance() {
		value := arr.CurrentValue()
		result = dispatcher.dispatchItem(
			ctx,
			value,
			func(s SelectorRunner) Decision { return s.SelectsFromIndex(index, negIndex) },
			r.followingSegment,
		)
		if !result {
			return
		}

		// Update the index
		index++
		if ahead != nil && !ahead.Advance() {
			negIndex++
		}

		if len(dispatcher.selectorStates) == 0 {
			break
		}
	}

	// Lastly if this is a descendant segment, we need to dive into items
	//
	// We do this here to comply with the order defined in the RFC but in a
	// streaming context it would be better to dive in inside the loop above.
	// Perhaps there could be a switch to decide whether we do this efficiently
	// or in a standard-compliant way.
	if r.isDescendantSegment {
		for arr2.Advance() {
			result = r.transformValue(ctx, arr2.CurrentValue(), next)
			if !result {
				return
			}
		}
	}

	return true
}

// itemDispatcher is a helper class for implementing the transformArray and
// transformObject methods of SegmentRunner.  It is able to process individual
// items in either type of collection.  A single itemDispatcher instance should
// be used to process a single collection.
//
// For a collection (object or array), given a specific item in the collection,
// it can decide what selectors in the segment select this item and either pass
// it on directly to the next processor in the pipeline if it's from the first
// active selector, or record it for later processing.
//
// Some mechanism like this is needed because we want to process the collection
// in a streaming way (item per item), but the jsonpath standard specifies that
// in the query $[S1, S2] all the items selected by S1 are emitted before the
// items selected by S2. So for example if the query is $[1,0] and the input is
// [1, 2] then the emitted items are 2, 1 in that order.
//
// It is also a valueProcessor because it is used in the next segment of the
// query as
type itemDispatcher struct {
	shouldClone    bool
	selectorStates []selectorState
	next           valueProcessor
}

var _ valueProcessor = &itemDispatcher{}

func newItemDispatcher(selectors []SelectorRunner, next valueProcessor) *itemDispatcher {
	selectorStates := make([]selectorState, len(selectors))
	for i, selector := range selectors {
		selectorStates[i].selector = selector
		selectorStates[i].reversesSelection = selector.ReversesSelection()
	}
	return &itemDispatcher{
		selectorStates: selectorStates,
		next:           next,
	}
}

// Flush the dispatcher and dispatch to d.next the pending items as long as result is true.
func (d *itemDispatcher) flush(ctx *RunContext, result bool) bool {
	for _, state := range d.selectorStates {
		result = state.flush(ctx, result, d.next)
	}
	return result
}

func (d *itemDispatcher) dispatchItem(ctx *RunContext, value iterator.Value, decide func(SelectorRunner) Decision, followingSegment *SegmentRunner) (result bool) {
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
		if followingSegment == nil {
			result = d.ProcessValue(ctx, value)
		} else {
			result = followingSegment.transformValue(ctx, value, d)
		}
	}
	return
}

func (d *itemDispatcher) ProcessValue(ctx *RunContext, value iterator.Value) bool {
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

type detachableValue struct {
	value      iterator.Value
	detachFunc func()
}

func (dv detachableValue) detach() {
	if dv.detachFunc != nil {
		dv.detachFunc()
	}
}
