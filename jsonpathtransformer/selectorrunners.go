package jsonpathtransformer

import "github.com/arnodel/jsonstream/iterator"

//
// Selector runners
//

// SelectorRunner is the interface used to run selectors.
type SelectorRunner interface {

	// Lookahead returns the number of extra number of array items we need to
	// know about in order to make a decision about a negative index selector.
	// If the returned number is 0 then it means there is no need to look ahead
	// at all.
	//
	// For example in the query $[-4:], the lookahead is 4.  This is because if
	// we know that there are at least 4 more items in the array after the
	// current one, it won't be selected.
	//
	// Knowing the lookahead value allows executing such queries in a streaming
	// manner, without loading the whole array into memory, just a slice of size
	// lookahead.
	Lookahead() int64

	// SelectsFromKey makes a Decision on whether the current object item is
	// selected based on its key.  If no decision can be made then it should
	// return DontKnow.
	SelectsFromKey(key string) Decision

	// SelectsFromKey makes a Decision on whether the current array item is
	// selected based on its index or negative index.  If no decision can be
	// made then it should return DontKnow.
	//
	// The value negIndex is negative and is meant to represent the index from
	// the end of the array, starting at -1.
	//
	// The negative index (negIndex) is not necessarily correct but the
	// following can be assumed, given that N is the value returned by
	// Lookahead() and I is the real negative index:
	//   - if negIndex >= -N then negIndex == I
	//   - if negIndex < -N then I < -N
	SelectsFromIndex(index, negIndex int64) Decision

	// Selects decides whether the current item in an object or array should be
	// selected, based on its value.  It is only called if either SelectsFromKey
	// or SelectsFromIndex returned DontKnow and it makes the final decision.
	//
	// Selects should promise not to advance Value, so it must clone it first if
	// it wants to look inside.
	SelectsFromValue(value iterator.Value) bool
}

// Here are the different kinds of SectorRunner.

var _ SelectorRunner = DefaultSelectorRunner{}
var _ SelectorRunner = NameSelectorRunner{}     // $.foo or $["foo"]
var _ SelectorRunner = WildcardSelectorRunner{} // $.* or $[*]
var _ SelectorRunner = IndexSelectorRunner{}    // $[42]
var _ SelectorRunner = SliceSelectorRunner{}    // $[:10] or $[-10:] or $[1::2]
var _ SelectorRunner = FilterSelectorRunner{}

// And here are their implmentations.

// DefaultSelectorRunner is the default implementation of SelectorRunner.  It
// selects nothing and is designed to be embedded in other implementation.
type DefaultSelectorRunner struct{}

// Lookahead returns 0.
func (r DefaultSelectorRunner) Lookahead() int64 {
	return 0
}

// SelectsFromIndex returns No.
func (r DefaultSelectorRunner) SelectsFromIndex(index, negIndex int64) Decision {
	return No
}

// SelectsFromKey returns No.
func (r DefaultSelectorRunner) SelectsFromKey(key string) Decision {
	return No
}

// SelectsFromValue returns false.
func (r DefaultSelectorRunner) SelectsFromValue(value iterator.Value) bool {
	return false
}

// NameSelectorRunner implements the SelectorRunner that selects a value in an
// object by the name of its key.
type NameSelectorRunner struct {
	DefaultSelectorRunner
	name string
}

// SelectsFromKey returns Yes if key is the name that can be selected, else No.
func (r NameSelectorRunner) SelectsFromKey(key string) Decision {
	return madeDecision(key == r.name)
}

// WildcardSelectorRunner implements the wildcard selector, which selects all
// values in an object and all items in an array.
type WildcardSelectorRunner struct {
	DefaultSelectorRunner
}

// SelectsFromIndex returns Yes
func (r WildcardSelectorRunner) SelectsFromIndex(index, negIndex int64) Decision {
	return Yes
}

// SelectsFromKey returns Yes
func (r WildcardSelectorRunner) SelectsFromKey(key string) Decision {
	return Yes
}

// IndexSelectorRunner implements the selector that selects an item at a given
// index in an array. That index can be negative (-1 means the last item, -2 the
// one before last etc).
type IndexSelectorRunner struct {
	DefaultSelectorRunner
	index int64
}

// Lookahead returns a non-zero value of the index of the selector is negative.
func (r IndexSelectorRunner) Lookahead() int64 {
	if r.index < 0 {
		return -r.index
	}
	return 0
}

func (r IndexSelectorRunner) SelectsFromIndex(index, negIndex int64) Decision {
	if r.index >= 0 {
		return madeDecision(index == r.index)
	} else {
		return madeDecision(negIndex == r.index)
	}
}

// SliceSelectorRunner implements the selector that selects a slice of an array.
// A slice is defined by 3 integer values start, end, step.  Start and end may
// be negative in which case they are counted from the end of the array starting
// from -1.
//
// Currently, negative steps are not implemented as it would require going through the
// array in reverse order.  TODO: support negative step
type SliceSelectorRunner struct {
	DefaultSelectorRunner
	start, end, step int64
}

// Lookahead returns a value that allows deciding whether we have reached the
// start or end index of the slice.
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

// SelectsFromIndex returns Yes or No, depending on whether the index is part of the slice.
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

type FilterSelectorRunner struct {
	DefaultSelectorRunner
	condition LogicalEvaluator
}

func (r FilterSelectorRunner) SelectsFromKey(key string) Decision {
	return DontKnow
}

func (r FilterSelectorRunner) SelectsFromIndex(index, negIndex int64) Decision {
	return DontKnow
}

func (r FilterSelectorRunner) SelectsFromValue(value iterator.Value) bool {
	return r.condition.Evaluate(value)
}

// Decision is a 3-valued type with possible values DontKnow, Yes, No.  Sort of
// a boolean for undecided people...
type Decision uint8

// IsMade is true if the Decision is not DontKnow
func (d Decision) IsMade() bool {
	return d != DontKnow
}

func madeDecision(yes bool) Decision {
	if yes {
		return Yes
	} else {
		return No
	}
}

// Possible values ofr Decision
const (
	DontKnow Decision = 0
	Yes      Decision = 1
	No       Decision = 2
)
