package jsonpathtransformer

import (
	"math"

	"github.com/arnodel/jsonstream/internal/jsonpath/ast"
	"github.com/arnodel/jsonstream/iterator"
	"github.com/arnodel/jsonstream/token"
)

type Compiler struct{}

func (c *Compiler) CompileQuery(query ast.Query) QueryRunner {
	segments := make([]SegmentRunner, len(query.Segments))
	for i, s := range query.Segments {
		segments[i] = c.CompileSegment(s)
	}
	switch query.RootNode {
	case ast.RootNodeIdentifier:
		return RootNodeQueryRunner{
			segments: segments,
		}
	case ast.CurrentNodeIdentifier:
		panic("unimplemented")
	default:
		panic("invalid query")
	}
}

func (c *Compiler) CompileSegment(segment ast.Segment) SegmentRunner {
	selectors := make([]SelectorRunner, len(segment.Selectors))
	var lookahead int64
	for i, s := range segment.Selectors {
		cs := c.CompileSelector(s)
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

func (c *Compiler) CompileSelector(selector ast.Selector) SelectorRunner {
	switch x := selector.(type) {
	case ast.NameSelector:
		return NameSelectorRunner{name: x.Name}
	case ast.WildcardSelector:
		return WildcardSelectorRunner{}
	case ast.IndexSelector:
		return IndexSelectorRunner{index: x.Index}
	case ast.FilterSelector:
		return FilterSelectorRunner{condition: c.CompileCondition(x.Condition)}
	case ast.SliceSelector:
		var start, end int64

		if x.Step < 0 {
			panic("unimplemented")
		}

		if x.Start == nil {
			start = 0
		} else {
			start = *x.Start
		}

		if x.End == nil {
			end = math.MaxInt64
		} else {
			end = *x.End
		}

		if end >= 0 && end <= start || start < 0 && start >= end {
			return DefaultSelectorRunner{}
		}
		return SliceSelectorRunner{
			start: start,
			end:   end,
			step:  x.Step,
		}
	default:
		panic("invalid selector")
	}
}

func (c *Compiler) CompileCondition(condition ast.LogicalExpr) LogicalEvaluator {
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
			Argument: c.CompileCondition(x.Argument),
		}
	case ast.ComparisonExpr:
		return c.CompileComparison(x)
	case ast.Query:
		panic("unimplemented")
	case ast.FunctionExpr:
		panic("unimplemented")
	default:
		panic("invalid condition")
	}
}

func (c *Compiler) compileConditions(conditions []ast.LogicalExpr) []LogicalEvaluator {
	return transformSlice(conditions, c.CompileCondition)
}

func (c *Compiler) CompileComparison(comparison ast.ComparisonExpr) ComparisonEvaluator {
	var flags ComparisonFlags
	left := c.CompileComparable(comparison.Left)
	right := c.CompileComparable(comparison.Right)
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

func (c *Compiler) CompileComparable(comparable ast.Comparable) ComparableEvaluator {
	switch x := comparable.(type) {
	case ast.Literal:
		scalar, err := token.ToScalar(x.Value)
		if err != nil {
			panic("invalid value in literal")
		}
		return LiteralEvaluator{value: (*iterator.Scalar)(scalar)}
	case ast.SingularQuery:
		return c.CompileSingularQuery(x)
	case ast.FunctionExpr:
		panic("unimplmemented")
	default:
		panic("invalid comparable type")
	}
}

func (c *Compiler) CompileSingularQuery(query ast.SingularQuery) CurrentNodeSingularQueryRunner {
	switch query.RootNode {
	case ast.CurrentNodeIdentifier:
		return CurrentNodeSingularQueryRunner{
			selectors: transformSlice(query.Segments, c.CompileSingularQuerySegment),
		}
	case ast.RootNodeIdentifier:
		panic("unimplemented")
	default:
		panic("invalid root node value")
	}
}

func (c *Compiler) CompileSingularQuerySegment(segment ast.SingularQuerySegment) SingularSelectorRunner {
	switch x := segment.(type) {
	case ast.NameSegment:
		return NameSingularSelectorRunner{nameSelector: NameSelectorRunner{name: x.Name}}
	case ast.IndexSegment:
		return IndexSingularSelectorRunner{indexSelector: IndexSelectorRunner{index: int64(x.Index)}}
	default:
		panic("invalid singular query segment type")
	}
}

func transformSlice[T, U any](slice []T, transform func(T) U) []U {
	result := make([]U, len(slice))
	for i, x := range slice {
		result[i] = transform(x)
	}
	return result
}
