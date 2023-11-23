package jsonpathtransformer

import (
	"math"

	"github.com/arnodel/jsonstream/internal/jsonpath/ast"
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
	switch segment.Type {
	case ast.ChildSegmentType:
		return ChildSegmentRunner{
			segmentRunnerBase: segmentRunnerBase{
				selectors: selectors,
				lookahead: lookahead,
			},
		}
	case ast.DescendantSegmentType:
		return DescendantSegmentRunner{
			segmentRunnerBase: segmentRunnerBase{
				selectors: selectors,
				lookahead: lookahead,
			},
		}
	default:
		panic("invalid segment")
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
		panic("unimplemented")
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
			return NothingSelectorRunner{}
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
