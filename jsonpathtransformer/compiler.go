package jsonpathtransformer

import (
	"errors"
	"fmt"
	"math"
	"reflect"

	"github.com/arnodel/jsonstream/internal/jsonpath/ast"
	"github.com/arnodel/jsonstream/iterator"
	"github.com/arnodel/jsonstream/token"
)

var ErrUnimplementedFeature = errors.New("unimplemented feature")

// CompileQuery compiles a JSON query AST to a QueryRunner.
func CompileQuery(query ast.Query) (MainQueryRunner, error) {
	c := compiler{
		functionRegistry: DefaultFunctionRegistry,
	}
	runner, err := c.compileQuery(query)
	if err != nil {
		return MainQueryRunner{}, err
	}
	singularQueries, queries := c.getInnerQueries()
	return MainQueryRunner{
		mainRunner:           runner,
		innerSingularQueries: singularQueries,
		innerQueries:         queries,
	}, nil
}

type innerSingularQueryEntry struct {
	query  ast.SingularQuery
	runner SingularQueryRunner
}

type innerQueryEntry struct {
	query  ast.Query
	runner QueryRunner
}

// This type may have some state or configuration parameters in the future.
type compiler struct {
	innerSingularQueries []innerSingularQueryEntry
	innerQueries         []innerQueryEntry

	functionRegistry FunctionRegistry
}

func (c *compiler) getInnerQueries() ([]SingularQueryRunner, []QueryRunner) {
	singularQueries := make([]SingularQueryRunner, len(c.innerSingularQueries))
	for i, e := range c.innerSingularQueries {
		singularQueries[i] = e.runner
	}
	queries := make([]QueryRunner, len(c.innerQueries))
	for i, e := range c.innerQueries {
		queries[i] = e.runner
	}
	return singularQueries, queries
}

func (c *compiler) compileQuery(query ast.Query) (r QueryRunner, err error) {
	segments := make([]SegmentRunner, len(query.Segments))
	for i, s := range query.Segments {
		segments[i], err = c.compileSegment(s)
		if err != nil {
			return
		}
	}
	r = QueryRunner{
		isRootNodeQuery: query.RootNode == ast.RootNodeIdentifier,
		segments:        segments,
	}
	return
}

func (c *compiler) compileInnerQueryCondition(query ast.Query) (LogicalEvaluator, error) {
	switch query.RootNode {
	case ast.CurrentNodeIdentifier:
		return c.compileQuery(query)
	case ast.RootNodeIdentifier:
		for i, entry := range c.innerQueries {
			if reflect.DeepEqual(entry.query, query) {
				return InnerQueryRunner{index: i}, nil
			}
		}
		q, err := c.compileQuery(query)
		if err != nil {
			return nil, err
		}
		c.innerQueries = append(c.innerQueries, innerQueryEntry{
			query:  query,
			runner: q,
		})
		return InnerQueryRunner{index: len(c.innerQueries) - 1}, nil
	default:
		panic("invalid query root node")
	}
}

func (c *compiler) compileSegment(segment ast.Segment) (r SegmentRunner, err error) {
	selectors := make([]SelectorRunner, len(segment.Selectors))
	var lookahead int64
	var cs SelectorRunner
	for i, s := range segment.Selectors {
		cs, err = c.compileSelector(s)
		if err != nil {
			return
		}
		selectors[i] = cs
		l := cs.Lookahead()
		if l > lookahead {
			lookahead = l
		}
	}
	r = SegmentRunner{
		selectors:           selectors,
		lookahead:           lookahead,
		isDescendantSegment: segment.Type == ast.DescendantSegmentType,
	}
	return
}

func (c *compiler) compileSelector(selector ast.Selector) (r SelectorRunner, err error) {
	switch x := selector.(type) {
	case ast.NameSelector:
		r = NameSelectorRunner{name: x.Name}
	case ast.WildcardSelector:
		r = WildcardSelectorRunner{}
	case ast.IndexSelector:
		r = IndexSelectorRunner{index: x.Index}
	case ast.FilterSelector:
		cond, err := c.compileCondition(x.Condition)
		if err != nil {
			return nil, err
		}
		r = FilterSelectorRunner{condition: cond}
	case ast.SliceSelector:
		r, err = c.compileSliceSelector(x)
	default:
		panic("invalid selector")
	}
	return
}

func (c *compiler) compileCondition(condition ast.LogicalExpr) (e LogicalEvaluator, err error) {
	switch x := condition.(type) {
	case ast.OrExpr:
		args, err := c.compileConditions(x.Arguments)
		if err != nil {
			return nil, err
		}
		e = LogicalOrEvaluator{Arguments: args}
	case ast.AndExpr:
		args, err := c.compileConditions((x.Arguments))
		if err != nil {
			return nil, err
		}
		e = LogicalAndEvaluator{Arguments: args}
	case ast.NotExpr:
		arg, err := c.compileCondition(x.Argument)
		if err != nil {
			return nil, err
		}
		e = LogicalNotEvaluator{
			Argument: arg,
		}
	case ast.ComparisonExpr:
		e, err = c.compileComparison(x)
	case ast.Query:
		e, err = c.compileInnerQueryCondition(x)
	case ast.FunctionExpr:
		e, err = c.compileFunctionExpr(x, LogicalType)
	default:
		panic("invalid condition")
	}
	return
}

func (c *compiler) compileConditions(conditions []ast.LogicalExpr) ([]LogicalEvaluator, error) {
	evs := make([]LogicalEvaluator, len(conditions))
	for i, cond := range conditions {
		ev, err := c.compileCondition(cond)
		if err != nil {
			return nil, err
		}
		evs[i] = ev
	}
	return evs, nil
}

func (c *compiler) compileSliceSelector(slice ast.SliceSelector) (r SelectorRunner, err error) {
	var start, end int64

	// I don't know yet how to support negative steps as it reverses the order of
	// the output.
	if slice.Step < 0 {
		return nil, fmt.Errorf("%w: negative slice step", ErrUnimplementedFeature)
	}

	// The spec says when the step is 0, no items are selected.
	if slice.Step == 0 {
		return DefaultSelectorRunner{}, nil
	}
	if slice.Start == nil {
		start = 0
	} else {
		start = *slice.Start
	}

	if slice.End == nil {
		// We'll never get such a big array, right?
		end = math.MaxInt64
	} else {
		end = *slice.End
	}

	if end >= 0 && end <= start || start < 0 && start >= end {
		// In this case, the slice selects nothing.
		r = DefaultSelectorRunner{}
	} else {
		r = SliceSelectorRunner{
			start: start,
			end:   end,
			step:  slice.Step,
		}
	}
	return
}

func (c *compiler) compileComparison(comparison ast.ComparisonExpr) (e ComparisonEvaluator, err error) {
	var flags ComparisonFlags
	var left, right ComparableEvaluator
	left, err = c.compileComparable(comparison.Left)
	if err != nil {
		return
	}
	right, err = c.compileComparable(comparison.Right)
	if err != nil {
		return
	}
	switch comparison.Op {
	case ast.EqualOp:
		flags = CheckEquals
	case ast.NotEqualOp:
		flags = CheckEquals | NegateResult
	case ast.LessThanOrEqualOp:
		flags = CheckEquals | CheckLessThan
	case ast.GreaterThanOrEqualOp:
		flags = CheckEquals | CheckLessThan
		left, right = right, left
	case ast.LessThanOp:
		flags = CheckLessThan
	case ast.GreaterThanOp:
		flags = CheckLessThan
		left, right = right, left
	default:
		panic("invalid comparison operator")
	}
	e = ComparisonEvaluator{
		left:  left,
		flags: flags,
		right: right,
	}
	return
}

func (c *compiler) compileComparable(comparable ast.Comparable) (ComparableEvaluator, error) {
	switch x := comparable.(type) {
	case ast.Literal:
		scalar, err := token.ToScalar(x.Value)
		if err != nil {
			panic("invalid value in literal")
		}
		return LiteralEvaluator{value: (*iterator.Scalar)(scalar)}, nil
	case ast.SingularQuery:
		return c.compileSingularQuery(x), nil
	case ast.FunctionExpr:
		return c.compileFunctionExpr(x, ValueType)
	default:
		panic("invalid comparable type")
	}
}

func (c *compiler) compileSingularQuery(query ast.SingularQuery) ComparableEvaluator {
	switch query.RootNode {
	case ast.CurrentNodeIdentifier:
		return SingularQueryRunner{
			selectors: mapSlice(query.Segments, c.compileSingularQuerySegment),
		}
	case ast.RootNodeIdentifier:
		for i, e := range c.innerSingularQueries {
			if reflect.DeepEqual(e.query, query) {
				return InnerSingularQueryRunner{index: i}
			}
		}
		q := SingularQueryRunner{
			selectors: mapSlice(query.Segments, c.compileSingularQuerySegment),
		}
		c.innerSingularQueries = append(c.innerSingularQueries, innerSingularQueryEntry{
			query:  query,
			runner: q,
		})
		return InnerSingularQueryRunner{index: len(c.innerSingularQueries) - 1}
	default:
		panic("invalid root node value")
	}
}

func (c *compiler) compileSingularQuerySegment(segment ast.SingularQuerySegment) SingularSelectorRunner {
	switch x := segment.(type) {
	case ast.NameSegment:
		return NameSingularSelectorRunner{nameSelector: NameSelectorRunner{name: x.Name}}
	case ast.IndexSegment:
		return IndexSingularSelectorRunner{indexSelector: IndexSelectorRunner{index: int64(x.Index)}}
	default:
		panic("invalid singular query segment type")
	}
}

func (c *compiler) compileFunctionExpr(f ast.FunctionExpr, returnType Type) (FunctionRunner, error) {
	def := c.functionRegistry.GetFunctionDef(f.FunctionName)
	if def == nil {
		return FunctionRunner{}, fmt.Errorf("unknown function %q", f.FunctionName)
	}
	// Check the function expr is well-typed
	if !def.OutputType.ConvertsTo(returnType) {
		return FunctionRunner{}, fmt.Errorf("expected type %s, got %s", returnType, def.OutputType)
	}
	if len(f.Arguments) != len(def.InputTypes) {
		return FunctionRunner{}, fmt.Errorf("expected %d arguments, got %d", len(def.InputTypes), len(f.Arguments))
	}
	args := make([]FunctionArgumentRunner, len(def.InputTypes))
	for i, expectedType := range def.InputTypes {
		arg, err := c.compileFunctionArg(f.Arguments[i], expectedType)
		if err != nil {
			return FunctionRunner{}, fmt.Errorf("argument %d: %w", i, err)
		}
		args[i] = arg
	}
	return FunctionRunner{
		FunctionDef: def,
		argRunners:  args,
	}, nil
}

func (c *compiler) compileFunctionArg(arg ast.FunctionArgument, expectedType Type) (FunctionArgumentRunner, error) {
	switch x := arg.(type) {
	case ast.Literal:
		if expectedType != ValueType {
			return nil, fmt.Errorf("expected %s, got literal value", expectedType)
		}
		cmp, err := c.compileComparable(x)
		if err != nil {
			return nil, err
		}
		return ValueArgumentRunner{ComparableEvaluator: cmp}, nil
	case ast.Query:
		switch expectedType {
		case ValueType:
			sq, ok := x.AsSingularQuery()
			if !ok {
				return nil, fmt.Errorf("expected %s, got a non-singular query", expectedType)
			}
			return ValueArgumentRunner{ComparableEvaluator: c.compileSingularQuery(sq)}, nil
		case NodesType:
			q, err := c.compileQuery(x)
			if err != nil {
				return nil, err
			}
			return NodesArgumentRunner{NodesResultEvaluator: q}, nil
		case LogicalType:
			return nil, fmt.Errorf("%w: converting query function argument to logical expr", ErrUnimplementedFeature)
		default:
			panic("invalid expected type")
		}
	case ast.LogicalExprArgument:
		switch expectedType {
		case NodesType, ValueType:
			return nil, fmt.Errorf("expected %s, got a logical expression", expectedType)
		case LogicalType:
			cond, err := c.compileCondition(x.LogicalExpr)
			if err != nil {
				return nil, err
			}
			return LogicalArgumentRunner{LogicalEvaluator: cond}, nil
		default:
			panic("invalid expected type")
		}
	case ast.FunctionExpr:
		return c.compileFunctionExpr(x, expectedType)
	default:
		panic("invalid function argument")
	}
}

func mapSlice[T, U any](slice []T, transform func(T) U) []U {
	result := make([]U, len(slice))
	for i, x := range slice {
		result[i] = transform(x)
	}
	return result
}
