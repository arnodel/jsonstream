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
	for i, s := range segment.Selectors {
		selectors[i] = c.CompileSelector(s)
	}
	switch segment.Type {
	case ast.ChildSegmentType:
		return ChildSegmentRunner{
			selectors: selectors,
		}
	case ast.DescendantSegmentType:
		return DescendantSegmentRunner{
			selectors: selectors,
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

		switch {
		case x.Start == nil:
			start = 0
		case *x.Start < 0:
			panic("unimplemented")
		default:
			start = *x.Start
		}

		switch {
		case x.End == nil:
			end = math.MaxInt64
		case *x.End < 0:
			panic("unimplemented")
		default:
			end = *x.End
		}

		if x.Step < 0 {
			panic("unimplemented")
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
