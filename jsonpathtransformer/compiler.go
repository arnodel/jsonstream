package jsonpathtransformer

import (
	"math"

	"github.com/arnodel/jsonstream/internal/jsonpath/ast"
	"github.com/arnodel/jsonstream/iterator"
	"github.com/arnodel/jsonstream/token"
)

// CompileQuery compiles a JSON query AST to a QueryRunner.
func CompileQuery(query ast.Query) QueryRunner {
	var c compiler
	return c.compileQuery(query)
}

// This type may have some state or configuration parameters in the future.
type compiler struct{}

func (c *compiler) compileQuery(query ast.Query) QueryRunner {
	segments := make([]SegmentRunner, len(query.Segments))
	for i, s := range query.Segments {
		segments[i] = c.compileSegment(s)
	}
	return QueryRunner{
		isRootNodeQuery: query.RootNode == ast.RootNodeIdentifier,
		segments:        segments,
	}
}

func (c *compiler) compileSegment(segment ast.Segment) SegmentRunner {
	selectors := make([]SelectorRunner, len(segment.Selectors))
	var lookahead int64
	for i, s := range segment.Selectors {
		cs := c.compileSelector(s)
		selectors[i] = cs
		l := cs.Lookahead()
		if l > lookahead {
			lookahead = l
		}
	}
	return SegmentRunner{
		selectors:           selectors,
		lookahead:           lookahead,
		isDescendantSegment: segment.Type == ast.DescendantSegmentType,
	}
}

func (c *compiler) compileSelector(selector ast.Selector) SelectorRunner {
	switch x := selector.(type) {
	case ast.NameSelector:
		return NameSelectorRunner{name: x.Name}
	case ast.WildcardSelector:
		return WildcardSelectorRunner{}
	case ast.IndexSelector:
		return IndexSelectorRunner{index: x.Index}
	case ast.FilterSelector:
		return FilterSelectorRunner{condition: c.compileCondition(x.Condition)}
	case ast.SliceSelector:
		return c.compileSliceSelector(x)
	default:
		panic("invalid selector")
	}
}

func (c *compiler) compileCondition(condition ast.LogicalExpr) LogicalEvaluator {
	switch x := condition.(type) {
	case ast.OrExpr:
		return LogicalOrEvaluator{
			Arguments: c.compileConditions(x.Arguments),
		}
	case ast.AndExpr:
		return LogicalAndEvaluator{
			Arguments: c.compileConditions(x.Arguments),
		}
	case ast.NotExpr:
		return LogicalNotEvaluator{
			Argument: c.compileCondition(x.Argument),
		}
	case ast.ComparisonExpr:
		return c.compileComparison(x)
	case ast.Query:
		q := c.compileQuery(x)
		if q.isRootNodeQuery {
			panic("root node query filter unimplemented")
		}
		return q
	case ast.FunctionExpr:
		panic("unimplemented")
	default:
		panic("invalid condition")
	}
}

func (c *compiler) compileConditions(conditions []ast.LogicalExpr) []LogicalEvaluator {
	return mapSlice(conditions, c.compileCondition)
}

func (c *compiler) compileSliceSelector(slice ast.SliceSelector) SelectorRunner {
	var start, end int64

	// I don't kow yet how to support negative steps as it reverses the order of
	// the output.
	if slice.Step < 0 {
		panic("unimplemented")
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
		return DefaultSelectorRunner{}
	}
	return SliceSelectorRunner{
		start: start,
		end:   end,
		step:  slice.Step,
	}
}

func (c *compiler) compileComparison(comparison ast.ComparisonExpr) ComparisonEvaluator {
	var flags ComparisonFlags
	left := c.compileComparable(comparison.Left)
	right := c.compileComparable(comparison.Right)
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
	return ComparisonEvaluator{
		left:  left,
		flags: flags,
		right: right,
	}
}

func (c *compiler) compileComparable(comparable ast.Comparable) ComparableEvaluator {
	switch x := comparable.(type) {
	case ast.Literal:
		scalar, err := token.ToScalar(x.Value)
		if err != nil {
			panic("invalid value in literal")
		}
		return LiteralEvaluator{value: (*iterator.Scalar)(scalar)}
	case ast.SingularQuery:
		return c.compileSingularQuery(x)
	case ast.FunctionExpr:
		panic("unimplmemented")
	default:
		panic("invalid comparable type")
	}
}

func (c *compiler) compileSingularQuery(query ast.SingularQuery) CurrentNodeSingularQueryRunner {
	switch query.RootNode {
	case ast.CurrentNodeIdentifier:
		return CurrentNodeSingularQueryRunner{
			selectors: mapSlice(query.Segments, c.compileSingularQuerySegment),
		}
	case ast.RootNodeIdentifier:
		panic("unimplemented")
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

func mapSlice[T, U any](slice []T, transform func(T) U) []U {
	result := make([]U, len(slice))
	for i, x := range slice {
		result[i] = transform(x)
	}
	return result
}
