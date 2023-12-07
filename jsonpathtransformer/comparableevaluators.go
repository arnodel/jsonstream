package jsonpathtransformer

import (
	"math"

	"github.com/arnodel/jsonstream/iterator"
)

type ComparableEvaluator interface {
	// The returned value must be cloned first if it is consumed
	Evaluate(ctx *RunContext, value iterator.Value) iterator.Value
}

var _ ComparableEvaluator = LiteralEvaluator{}
var _ ComparableEvaluator = SingularQueryRunner{}
var _ ComparableEvaluator = InnerSingularQueryRunner{}

type LiteralEvaluator struct {
	value iterator.Value
}

func (e LiteralEvaluator) Evaluate(ctx *RunContext, value iterator.Value) iterator.Value {
	return e.value
}

//
// Singular query runner
//

type SingularQueryRunner struct {
	selectors []SingularSelectorRunner
}

func (r SingularQueryRunner) Evaluate(ctx *RunContext, value iterator.Value) iterator.Value {
	var detach func()
	value, detach = value.Clone()
	if detach != nil {
		defer detach()
	}
	for _, selector := range r.selectors {
		switch x := value.(type) {
		case *iterator.Object:
			value = selector.SelectFromObject(x)
		case *iterator.Array:
			value = selector.SelectFromArray(x)
		}
		if value == nil {
			break
		}
	}
	return value
}

type SingularSelectorRunner interface {
	SelectFromObject(*iterator.Object) iterator.Value
	SelectFromArray(*iterator.Array) iterator.Value
}

var _ SingularSelectorRunner = DefaultSingularSelectorRunner{}
var _ SingularSelectorRunner = NameSingularSelectorRunner{}
var _ SingularSelectorRunner = IndexSingularSelectorRunner{}

type DefaultSingularSelectorRunner struct{}

func (r DefaultSingularSelectorRunner) SelectFromObject(*iterator.Object) iterator.Value {
	return nil
}

func (r DefaultSingularSelectorRunner) SelectFromArray(*iterator.Array) iterator.Value {
	return nil
}

type NameSingularSelectorRunner struct {
	DefaultSingularSelectorRunner
	nameSelector NameSelectorRunner
}

func (r NameSingularSelectorRunner) SelectFromObject(obj *iterator.Object) iterator.Value {
	for obj.Advance() {
		key, value := obj.CurrentKeyVal()
		if r.nameSelector.SelectsFromKey(keyStringValue(key)) == Yes {
			return value
		}
	}
	return nil
}

type IndexSingularSelectorRunner struct {
	DefaultSingularSelectorRunner
	indexSelector IndexSelectorRunner
}

func (r IndexSingularSelectorRunner) SelectFromArray(arr *iterator.Array) iterator.Value {
	lookahead := r.indexSelector.Lookahead()
	var index, negIndex int64
	var ahead *iterator.Array

	if lookahead > 0 {
		var detach func()
		ahead, detach = arr.CloneArray()
		defer detach()
		for negIndex+lookahead >= 0 && ahead.Advance() {
			negIndex--
		}
	} else {
		negIndex = math.MinInt64
	}

	for arr.Advance() {
		value := arr.CurrentValue()
		if r.indexSelector.SelectsFromIndex(index, negIndex) == Yes {
			return value
		}
		index++
		if ahead != nil && !ahead.Advance() {
			negIndex++
		}
	}
	return nil
}
